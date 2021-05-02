package net

import (
	"bytes"
	"context"
	"encoding/binary"
	"io/ioutil"
	"sort"
	"time"

	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/helpers"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/libp2p/go-libp2p-core/record"
	protoutil "github.com/wetware/casm/pkg/util/proto"
)

/*
 * This file contains the implementation for the random walk that subtends Overlay.FindPeers.
 *
 * The algorithm is remarkably simple, even though the unholy mix of distribution and concurrency
 * makes its implementation quite verbose.  A random walk works like this:
 *
 *  1. FindPeers selects a depth parameter, corresponding to the number of steps in the random walk.  The default is 9.
 *  2. FindPeers selects a neighbor at random, opens a stream to the /casm/net/sample endpoint, and transmits depth-1.
 *  3. The remote peer receives the depth parameter.  If depth == 1, it replies with the address of one of its neighbors at random.†
 *  4. Else depth > 1, and the remote peer picks up the random walk at step #2 (recursion).
 *
 * That's it. Everything else is glue code.
 *
 * When reading over this code, you should skip over anything you don't understand.  Don't shave the proverbial yak.
 *
 * † A quick explanation, in case 'depth == 1' surprised you.  We random walk until 1 instead of 0 to avoid an extra round-trip
 *   on the network. When we get to the penultimate node, we need only pick one of its neighbors at random and return its listen
 *   address.  We do not need to open a stream to it.
 */

const (
	ProtocolVersion             = "0.0.0"
	BaseProto       protocol.ID = "/casm/net"

	defaultSampleLimit       = 1
	defaultSampleDepth       = 7
	defaultSampleStepTimeout = time.Second * 30
)

var (
	// sampleProto and joinProto are not used for routing.  Their only
	// purpose is to serve as a key when (un)registering protocol handlers.
	//
	// They should never appear on the wire.
	joinProto   = protoutil.Join(BaseProto, "join")
	sampleProto = protoutil.Join(BaseProto, "sample")
	gossipProto = protoutil.Join(BaseProto, "gossip")

	versionProto protocol.ID = protoutil.Join(BaseProto, protocol.ID(ProtocolVersion))
	matchVersion func(string) bool
)

func init() {
	var err error
	if matchVersion, err = helpers.MultistreamSemverMatcher(
		protoutil.Join(BaseProto, protocol.ID(ProtocolVersion)),
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

func (o *Overlay) matchSample(s string) bool {
	//	/casm/net/<version>/<ns>/sample
	base, subproto := protoutil.Split(protocol.ID(s))
	if subproto != "sample" {
		return false
	}

	//	/casm/net/<version>/<ns>
	return o.matchJoin(string(base))
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
 * Random-Walk implementation (sample).
 */

type (
	deliverer interface {
		Deliver(context.Context, *peer.PeerRecord) error
	}

	stepper interface {
		Step(context.Context) error
	}

	streamDialer interface {
		NewStream(context.Context, peer.ID, ...protocol.ID) (network.Stream, error)
	}
)

type sampleOpts discovery.Options

func (os *sampleOpts) Apply(opt ...discovery.Option) error {
	return (*discovery.Options)(os).Apply(opt...)
}

func (os *sampleOpts) Breadth() int {
	if i := (*discovery.Options)(os).Limit; i > 0 {
		return i
	}

	return defaultSampleLimit
}

func (os *sampleOpts) Depth() uint8 {
	if os.Other == nil {
		return defaultSampleDepth
	}

	if d, ok := os.Other[keyDepth]; ok {
		return d.(uint8)
	}

	return defaultSampleDepth
}

type randWalk struct {
	depth uint8
	sd    streamDialer
	peer  peer.ID
	d     deliverer
	proto protocol.ID
}

func (w randWalk) Step(ctx context.Context) error {
	s, err := w.sd.NewStream(ctx, w.peer, w.proto)
	if err != nil {
		return err
	}
	defer s.Close()

	return w.next(ctx, s)
}

func (w randWalk) next(ctx context.Context, s network.Stream) error {
	if err := w.sendPayload(ctx, s); err != nil {
		return err
	}

	return w.awaitResult(ctx, s)
}

func (w randWalk) sendPayload(ctx context.Context, s network.Stream) error {
	if err := w.setWriteDeadline(ctx, s); err != nil {
		return err
	}

	return binary.Write(s, binary.BigEndian, w.nextStep(ctx))
}

func (w randWalk) setWriteDeadline(ctx context.Context, s network.Stream) error {
	if t, ok := ctx.Deadline(); ok {
		return s.SetWriteDeadline(t)
	}

	return nil
}

func (w randWalk) nextStep(ctx context.Context) *sampleStep {
	s := sampleStep{Depth: w.depth - 1}
	if t, ok := ctx.Deadline(); ok {
		s.Timeout = time.Until(t)
	}
	return &s
}

func (w randWalk) awaitResult(ctx context.Context, s network.Stream) error {
	if err := w.setReadDeadline(ctx, s); err != nil {
		return err
	}

	return w.readEnvelope(ctx, s)
}

func (w randWalk) readEnvelope(ctx context.Context, s network.Stream) error {
	b, err := w.readPayload(ctx, s)
	if err != nil {
		return err
	}

	return w.deliver(ctx, b)
}

func (w randWalk) readPayload(ctx context.Context, s network.Stream) ([]byte, error) {
	if err := w.setReadDeadline(ctx, s); err != nil {
		return nil, err
	}

	return ioutil.ReadAll(s)
}

func (w randWalk) setReadDeadline(ctx context.Context, s network.Stream) error {
	if t, ok := ctx.Deadline(); ok {
		return s.SetReadDeadline(t)
	}

	return nil
}

func (w randWalk) deliver(ctx context.Context, b []byte) error {
	_, rec, err := record.ConsumeEnvelope(b, peer.PeerRecordEnvelopeDomain)
	if err != nil {
		return err
	}

	return w.d.Deliver(ctx, rec.(*peer.PeerRecord))
}

type sampleStep struct {
	Depth   uint8
	Timeout time.Duration
}

func (s sampleStep) Deadline() time.Time {
	if s.Timeout == 0 {
		return time.Now().Add(defaultSampleStepTimeout)
	}

	return time.Now().Add(s.Timeout)
}

func (ss *sampleStep) RecvPayload(s network.Stream) error {
	if err := s.SetReadDeadline(ss.Deadline()); err != nil {
		return err
	}

	return binary.Read(s, binary.BigEndian, ss)
}

func (ss *sampleStep) Next(s network.Stream, h host.Host, peer peer.ID) stepper {
	if ss.Depth > 1 {
		return randWalk{
			peer:  peer,
			depth: ss.Depth - 1,
			d:     deliveryStream{s: s, h: h},
			sd:    h,
			proto: s.Protocol(),
		}
	}

	return randChoice{
		peer: peer,
		d:    deliveryStream{s: s, h: h},
		h:    h,
	}
}

type randChoice struct {
	peer peer.ID
	d    deliverer
	h    host.Host
}

func (c randChoice) Step(ctx context.Context) error {
	return c.d.Deliver(ctx, &peer.PeerRecord{
		PeerID: c.peer,
		Addrs:  c.h.Peerstore().Addrs(c.peer),
	})
}

type gossip struct {
	n     *neighborhood
	sd    streamDialer
	h     host.Host
	peer  peer.ID
	proto protocol.ID
}

func (g gossip) PushPull(ctx context.Context) ([]*peer.PeerRecord, error) {
	s, err := g.sd.NewStream(ctx, g.peer, g.proto)
	if err != nil {
		return nil, err
	}
	defer s.Close()

	if err = g.Push(ctx, s); err != nil {
		return nil, err
	}
	return g.Pull(ctx, s)
}

func (g gossip) Push(ctx context.Context, s network.Stream) error {
	peers := g.n.Peers()
	peerAmount := len(peers)
	err := binary.Write(s, binary.BigEndian, peerAmount)
	if err != nil {
		return err
	}
	for _, rec := range peers {
		rec.Addrs = g.h.Peerstore().Addrs(rec.PeerID)
		err = g.sendRecord(ctx, s, rec)
		if err != nil {
			return err
		}
	}
	return nil
}

func (g gossip) Pull(ctx context.Context, s network.Stream) ([]*peer.PeerRecord, error) {
	// receive records
	var amount int
	err := binary.Read(s, binary.BigEndian, &amount)
	if err != nil {
		return nil, err
	}
	peers := make([]*peer.PeerRecord, amount)
	for i := 0; i < amount; i++ {
		rec, err := g.recvRecord(s)
		if err != nil {
			return nil, err
		}
		peers[i] = rec
	}
	// merge with local neighbors and filter out best peers
	peers = append(peers, g.n.Peers()...)
	sort.Slice(peers, func(i, j int) bool {
		return peers[i].Seq > peers[j].Seq
	})
	return peers[:g.n.MaxSize()], nil
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
