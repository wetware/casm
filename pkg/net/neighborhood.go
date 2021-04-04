package net

import (
	"bytes"
	"context"
	"io"
	"math/rand"
	"sync"
	"sync/atomic"

	"github.com/jbenet/goprocess"
	"github.com/libp2p/go-eventbus"
	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/record"
	ctxutil "github.com/lthibault/util/ctx"
)

type neighborhood struct {
	log Logger

	vtx vertex

	h     host.Host
	state event.Emitter
	proc  goprocess.Process

	gcCtr sync.WaitGroup
	gc    chan struct{}
	lease chan leaseRequest
	evict chan peer.ID
}

func (n *neighborhood) init(log Logger, h host.Host) (err error) {
	n.state, err = h.EventBus().Emitter(new(EvtState), eventbus.Stateful)
	if err != nil {
		return
	}

	n.h = h
	n.log = log.With(n)
	n.gc = make(chan struct{}, 1)
	n.lease = make(chan leaseRequest, 1)
	n.evict = make(chan peer.ID, 1)

	n.proc = h.Network().Process().Go(n.loop)
	n.proc.SetTeardown(func() error {
		n.gcCtr.Wait()
		close(n.lease)
		close(n.evict)
		return n.state.Close()
	})

	return
}

func (n *neighborhood) Close() error { return n.proc.Close() }
func (n *neighborhood) Context() context.Context {
	return ctxutil.FromChan(n.proc.Closing())
}

func (n *neighborhood) Neighbors() peer.IDSlice {
	vtx := n.vtx.Load()
	ns := make(peer.IDSlice, 0, len(vtx))
	for id := range vtx {
		ns = append(ns, id)
	}
	return ns
}

func (n *neighborhood) RandPeer() (peer.ID, error) {
	if e, ok := n.vtx.Random(); ok {
		return e.ID(), nil
	}

	return "", ErrNoPeers
}

func (n *neighborhood) Loggable() map[string]interface{} {
	return map[string]interface{}{
		"type":    "casm.net.neighborhood",
		"n_peers": len(n.vtx.Load()),
	}
}

func (n *neighborhood) loop(p goprocess.Process) {
	for {
		select {
		case <-p.Closing():
			return

		case req := <-n.lease:
			n.handleLease(req)

		case id := <-n.evict:
			n.handleEvict(id)

		case <-n.gc:
			n.handleGC()
		}
	}
}

func (n *neighborhood) Lease(ctx context.Context, s network.Stream) (context.Context, error) {
	select {
	case <-n.Context().Done():
		return nil, nil
	default:
	}

	req := newLeaseRequest(s)

	select {
	case <-n.Context().Done():
		return nil, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case n.lease <- req:
		return req.Wait(ctx)
	}
}

func (n *neighborhood) Evict(id peer.ID) {
	select {
	case <-n.Context().Done():
	default:
		select {
		case <-n.Context().Done():
		case n.evict <- id:
		}
	}
}

func (n *neighborhood) handleLease(req leaseRequest) {
	defer close(req.done)
	defer close(req.err)
	e := req.NewEdge()

	if !n.vtx.AddEdge(e) {
		req.err <- req.s.Close()
		return
	}

	go func() {
		defer close(e.done)
		_, _ = io.Copy(io.Discard, req.s)
	}()

	req.done <- ctxutil.FromChan(e.done)
}

func (n *neighborhood) handleEvict(id peer.ID) {
	ok, err := n.vtx.RemoveEdge(id)
	if !ok {
		return
	}

	if err != nil {
		n.log.WithField("peer", id).WithError(err).Error("error closing edge")
	}

	n.emit(id, EventLeft)
}

func (n *neighborhood) handleGC() {
	es := n.vtx.Load()

	// did copy drop any (disconnected) edges?
	if new := es.Copy(); len(es) != len(new) {
		n.vtx.Store(new)
	}
}

func (n *neighborhood) emit(id peer.ID, ev Event) {
	if err := n.state.Emit(EvtState{
		Peer:  id,
		Event: ev,
	}); err != nil {
		n.log.WithError(err).With(ev).Error("failed to emit event")
	}
}

func (n *neighborhood) Handle(s network.Stream) {
	var ss sampleStep
	if err := ss.RecvPayload(s); err != nil {
		n.log.WithStream(s).
			WithError(err).
			Debug("error encountered during random walk (recv payload)")
		return
	}

	// HACK:  the local peer can become orphaned during a walk
	//		  if the join stream terminates concurrently.  In
	//		  such cases, we pretend the remote peer is still a
	//		  member of the neighborhood, and proceed as planned.
	peer := s.Conn().RemotePeer()
	if e, ok := n.vtx.Random(); ok {
		peer = e.ID()
	}

	ctx, cancel := context.WithDeadline(n.Context(), ss.Deadline())
	defer cancel()

	if err := ss.Next(s, n.h, peer).Step(ctx); err != nil {
		n.log.WithStream(s).
			WithError(err).
			Debug("error encountered during random walk (next step)")
	}
}

type vertex struct {
	r     *rand.Rand
	value atomic.Value
}

func (vtx *vertex) AddEdge(e edge) bool {
	es := vtx.Load()
	if _, ok := es[e.ID()]; ok {
		return false
	}

	es = es.Copy()
	es[e.ID()] = e
	vtx.Store(es)
	return true
}

func (vtx *vertex) RemoveEdge(id peer.ID) (bool, error) {
	if e, ok := vtx.Get(id); ok {
		return true, e.Close()
	}

	return false, nil
}

func (vtx *vertex) Random() (edge, bool) {
	es := vtx.Load()
	if len(es) == 0 {
		return edge{}, false
	}

	slice := make([]edge, 0, len(es))

	for _, e := range es {
		select {
		case <-e.done:
		default:
			slice = append(slice, e)
		}
	}

	vtx.r.Shuffle(len(slice), func(i, j int) {
		slice[i], slice[j] = slice[j], slice[i]
	})

	return slice[0], true
}

func (vtx *vertex) Get(id peer.ID) (edge, bool) {
	if e, ok := vtx.Load()[id]; ok {
		select {
		case <-e.done:
		default:
			return e, ok
		}
	}
	return edge{}, false
}

func (vtx *vertex) Load() edgeMap {
	if v := vtx.value.Load(); v != nil {
		return v.(edgeMap)
	}

	return nil
}

func (vtx *vertex) Store(es edgeMap) { vtx.value.Store(es) }

type edge struct {
	s    network.Stream
	done chan struct{}
}

func (e edge) ID() peer.ID { return e.s.Conn().RemotePeer() }

func (e edge) Close() error {
	select {
	case <-e.done:
	default:
		close(e.done)
	}
	return e.s.Reset()
}

type edgeMap map[peer.ID]edge

func (m edgeMap) Copy() edgeMap {
	new := make(edgeMap, len(m))
	for id, e := range m {
		select {
		case <-e.done:
		default:
			new[id] = e
		}
	}
	return new
}

type deliveryStream struct {
	s network.Stream
	h host.Host
}

func (d deliveryStream) Deliver(ctx context.Context, rec *peer.PeerRecord) error {
	env, err := record.Seal(rec, d.h.Peerstore().PrivKey(d.h.ID()))
	if err != nil {
		return err
	}

	b, err := env.Marshal()
	if err != nil {
		return err
	}

	if t, ok := ctx.Deadline(); ok {
		if err = d.s.SetWriteDeadline(t); err != nil {
			return err
		}
	}

	if _, err = io.Copy(d.s, bytes.NewReader(b)); err != nil {
		return err
	}

	return d.s.CloseWrite()
}

type leaseRequest struct {
	done chan context.Context
	s    network.Stream
	err  chan error
}

func newLeaseRequest(s network.Stream) leaseRequest {
	return leaseRequest{
		s:    s,
		done: make(chan context.Context, 1),
		err:  make(chan error, 1),
	}
}

func (req leaseRequest) NewEdge() edge {
	return edge{
		s:    req.s,
		done: make(chan struct{}),
	}
}

func (req leaseRequest) Wait(ctx context.Context) (context.Context, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case ctx = <-req.done:
		return ctx, nil
	case err := <-req.err:
		if err == nil {
			return nil, errEdgeExists
		}
		return nil, err
	}
}
