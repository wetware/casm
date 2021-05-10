package net

import (
	"bytes"
	"context"
	"encoding/binary"
	"sort"

	"github.com/libp2p/go-libp2p-core/helpers"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/libp2p/go-libp2p-core/record"
	protoutil "github.com/wetware/casm/pkg/util/proto"
	"golang.org/x/sync/errgroup"
)

/*
 * TODO: Write Gossiping short explanation?
 */

const (
	ProtocolVersion             = "0.0.0"
	BaseProto       protocol.ID = "/casm/net"
)

var (
	// joinProto is not used for routing.  The only
	// purpose is to serve as a key when (un)registering protocol handlers.
	//
	// They should never appear on the wire.
	joinProto   = protoutil.Join(BaseProto, "join")
	gossipProto = protoutil.Join(BaseProto, "gossip")

	versionProto = protoutil.Join(BaseProto, ProtocolVersion)
	matchVersion func(string) bool
)

func init() {
	var err error
	if matchVersion, err = helpers.MultistreamSemverMatcher(
		protoutil.Join(BaseProto, ProtocolVersion),
	); err != nil {
		panic(err)
	}
}

/*
 * Protocol Routing
 */

func (o *Overlay) matchJoin(s string) bool {
	//	/casm/net/<version>/<ns>
	base, subproto := protoutil.Split(protocol.ID(s))
	if matchVersion(string(base)) && string(subproto) == o.ns {
		return true
	}

	return false
}

func (o *Overlay) matchGossip(s string) bool {
	//	/casm/net/<version>/<ns>/gossip
	base, subproto := protoutil.Split(protocol.ID(s))
	if subproto != "gossip" {
		return false
	}

	//	/casm/net/<version>/<ns>
	return o.matchJoin(string(base))
}

/*
 * Gossiping implementation.
 */

type gossiper struct {
	n *neighborhood
	h host.Host
	s network.Stream
	r *atomicRand
}

func (g gossiper) PushPull(ctx context.Context) (recordSlice, error) {
	gr, ctx := errgroup.WithContext(ctx)

	var recs recordSlice
	gr.Go(func() error {
		return g.push(ctx)
	})
	gr.Go(func() (err error) {
		recs, err = g.pull(ctx)
		return err
	})
	return recs, gr.Wait()
}

func (g gossiper) push(ctx context.Context) error {
	recs := g.n.Records()
	err := binary.Write(g.s, binary.BigEndian, len(recs))
	if err != nil {
		return err
	}
	for _, rec := range recs {
		err = g.sendRecord(ctx, g.s, rec)
		if err != nil {
			return err
		}
	}
	return nil
}

func (g gossiper) pull(ctx context.Context) (recordSlice, error) {
	var amount int
	err := binary.Read(g.s, binary.BigEndian, &amount)
	if err != nil {
		return nil, err
	}
	recs := make(recordSlice, amount)
	for i := 0; i < amount; i++ {
		rec, err := g.recvRecord(g.s)
		if err != nil {
			return nil, err
		}
		recs[i] = rec
	}
	recs = append(recs, g.n.Records()...)
	sort.Sort(recs)
	if peersAreNear(g.h.ID(), g.s.Conn().RemotePeer()) {
		g.r.Shuffle(len(recs), func(i, j int) {
			recs[i], recs[j] = recs[j], recs[i]
		})
	}
	return recs[:g.n.MaxLen()], nil
}

func (g gossiper) sendRecord(ctx context.Context, s network.Stream, rec *peer.PeerRecord) error {
	env, err := record.Seal(rec, g.h.Peerstore().PrivKey(g.h.ID()))
	if err != nil {
		return err
	}

	b, err := env.Marshal()
	if err != nil {
		return err
	}

	if t, ok := ctx.Deadline(); ok {
		if err = s.SetWriteDeadline(t); err != nil {
			return err
		}
	}

	return binary.Write(s, binary.BigEndian, b)
}

func (g gossiper) recvRecord(s network.Stream) (*peer.PeerRecord, error) {
	buf := new(bytes.Buffer)
	err := binary.Read(s, binary.BigEndian, &buf)
	if err != nil {
		return nil, err
	}
	_, rec, err := record.ConsumeEnvelope(buf.Bytes(), peer.PeerRecordEnvelopeDomain)
	if err != nil {
		return nil, err
	}
	rec, ok := rec.(*peer.PeerRecord)
	if !ok {

		return nil, err
	}
	return rec.(*peer.PeerRecord), nil
}
