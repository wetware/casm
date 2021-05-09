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
 * Gossiping implementation (gossip).
 */

type (
	streamDialer interface {
		NewStream(context.Context, peer.ID, ...protocol.ID) (network.Stream, error)
	}
)

type gossip struct {
	n      *neighborhood
	sd     streamDialer
	h      host.Host
	peerID peer.ID
	proto  protocol.ID
	r      *atomicRand
}

func (g gossip) PushPull(ctx context.Context) (recordSlice, error) {
	s, err := g.sd.NewStream(ctx, g.peerID, g.proto)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	gr, ctx := errgroup.WithContext(ctx)

	var recs recordSlice
	gr.Go(func() error {
		return g.Push(ctx, s)
	})
	gr.Go(func() error {
		var err error
		recs, err = g.Pull(ctx, s)
		return err
	})
	err = gr.Wait()
	return recs, err
}

func (g gossip) Push(ctx context.Context, s network.Stream) error {
	recs := g.n.Records()
	err := binary.Write(s, binary.BigEndian, len(recs))
	if err != nil {
		return err
	}
	for _, rec := range recs {
		rec.Addrs = g.h.Peerstore().Addrs(rec.PeerID)
		err = g.sendRecord(ctx, s, rec)
		if err != nil {
			return err
		}
	}
	return nil
}

func (g gossip) Pull(ctx context.Context, s network.Stream) (recordSlice, error) {
	var amount int
	err := binary.Read(s, binary.BigEndian, &amount)
	if err != nil {
		return nil, err
	}
	recs := make(recordSlice, amount)
	for i := 0; i < amount; i++ {
		rec, err := g.recvRecord(s)
		if err != nil {
			return nil, err
		}
		recs[i] = rec
	}
	recs = append(recs, g.n.Records()...)
	sort.Sort(recs)
	if peersAreNear(g.h.ID(), g.peerID) {
		g.r.Shuffle(len(recs), func(i, j int) {
			recs[i], recs[j] = recs[j], recs[i]
		})
	}
	return recs[:g.n.MaxLen()], nil
}

func (g gossip) sendRecord(ctx context.Context, s network.Stream, rec *peer.PeerRecord) error {
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

func (g gossip) recvRecord(s network.Stream) (*peer.PeerRecord, error) {
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
