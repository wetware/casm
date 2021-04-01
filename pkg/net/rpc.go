package net

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"time"

	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/libp2p/go-libp2p-core/record"
	syncutil "github.com/lthibault/util/sync"
)

const (
	BaseProto   protocol.ID = "/casm/mesh"
	ProtoJoin               = BaseProto + "/join"
	ProtoSample             = BaseProto + "/sample"
)

type endpoint interface {
	Protocol() protocol.ID
	Handle(Logger, network.Stream) error
}

type method interface {
	Call(context.Context, peer.AddrInfo) error
}

// type handler interface {
// 	Handle(network.Stream)
// }

type dialer interface {
	Connect(context.Context, peer.AddrInfo) error
	NewStream(context.Context, peer.ID, ...protocol.ID) (network.Stream, error)
}

type joiner Overlay

func (*joiner) Protocol() protocol.ID { return ProtoJoin }

func (j *joiner) Handle(_ Logger, s network.Stream) error {
	block(s)
	return nil
}

func (j *joiner) Call(ctx context.Context, info peer.AddrInfo) error {
	s, err := addrDialer(info).Dial(ctx, j.h, ProtoJoin)
	if err != nil {
		return err
	}

	return s.Close()
}

type handleSample Overlay

func (*handleSample) Protocol() protocol.ID { return ProtoSample }

func (h *handleSample) Handle(log Logger, s network.Stream) error {
	var depth uint8
	if err := binary.Read(s, binary.BigEndian, &depth); err != nil {
		return err
	}

	if depth == 0 {
		return h.returnResult(s, *host.InfoFromHost(h.h))
	}

	return h.walk(log, s, depth-1)
}

func (h *handleSample) walk(log Logger, s network.Stream, depth uint8) error {
	info, err := h.randomPeer(log)
	if err != nil {
		return err
	}

	ch := make(chan peer.AddrInfo, 1)

	any, ctx := syncutil.AnyWithContext(context.Background())
	any.Go((*Overlay)(h).call(ctx, info, sampleMethod{
		opt: discovery.Options{Limit: 1},
		d:   h.h,
		ch:  ch,
	}))

	// abort if the upstream connection times out.
	any.Go(func() error {
		block(s)
		return context.DeadlineExceeded // simulate a context bound to 's'
	})

	select {
	case <-ctx.Done():
		return ctx.Err()
	case info := <-ch:
		return h.returnResult(s, info)
	}
}

func (h *handleSample) returnResult(s network.Stream, info peer.AddrInfo) error {
	rec := peer.PeerRecord{PeerID: h.h.ID(), Addrs: h.h.Addrs()}
	env, err := record.Seal(&rec, h.h.Peerstore().PrivKey(h.h.ID()))
	if err != nil {
		return err
	}

	bs, err := env.Marshal()
	if err != nil {
		return err
	}

	_, err = io.Copy(s, bytes.NewBuffer(bs))
	return err
}

func (h *handleSample) randomPeer(log Logger) (peer.AddrInfo, error) {
	ns := h.stat.Load().View().Shuffle()

	// TODO:  can the neighborhood be empty here?  Investigate.
	if len(ns) == 0 {
		log.Warn("empty neighborhod on sub-sample")
		return peer.AddrInfo{}, errors.New("fixme") // XXX
	}

	return ns[0], nil
}

type sampleMethod struct {
	d   dialer
	opt discovery.Options
	ch  chan<- peer.AddrInfo
}

func (m sampleMethod) Call(ctx context.Context, info peer.AddrInfo) error {
	defer close(m.ch)

	s, err := addrDialer(info).Dial(ctx, m.d, ProtoSample)
	if err != nil {
		return err
	}
	defer s.Close()

	// ensure write deadline; default 30s.
	if t, ok := ctx.Deadline(); ok {
		s.SetWriteDeadline(t)
	} else {
		// belt-and-suspenders.  'ctx' should always have a deadline.
		s.SetWriteDeadline(time.Now().Add(m.opt.Ttl))
	}

	depth := m.opt.Other[depthOptKey{}].(uint8)
	if err = binary.Write(s, binary.BigEndian, depth-1); err != nil {
		return err
	}

	if err = s.CloseWrite(); err != nil {
		return err
	}

	var buf bytes.Buffer
	if _, err = io.Copy(&buf, s); err != nil {
		return err
	}

	_, urec, err := record.ConsumeEnvelope(buf.Bytes(), peer.PeerRecordEnvelopeDomain)
	if err != nil {
		return err
	}

	rec := urec.(*peer.PeerRecord)

	select {
	case m.ch <- peer.AddrInfo{ID: rec.PeerID, Addrs: rec.Addrs}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type addrDialer peer.AddrInfo

func (sd addrDialer) Dial(ctx context.Context, d dialer, ps ...protocol.ID) (network.Stream, error) {
	if err := d.Connect(ctx, peer.AddrInfo(sd)); err != nil {
		return nil, err
	}

	return d.NewStream(ctx, sd.ID, ps...)
}

// wait for a reader to close by blocking on a 'Read'
// call and discarding any data/error.
func block(r io.Reader) {
	var buf [1]byte
	r.Read(buf[:])
}
