package net

import (
	"context"
	"encoding/binary"
	"io/ioutil"
	"time"

	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/libp2p/go-libp2p-core/record"
)

const (
	ProtocolID protocol.ID = "/casm/net"
	SampleID               = ProtocolID + "/sample"

	defaultSampleLimit       = 1
	defaultSampleDepth       = 7
	defaultSampleStepTimeout = time.Second * 30
)

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
}

func (w randWalk) Step(ctx context.Context) error {
	s, err := w.sd.NewStream(ctx, w.peer, SampleID)
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
