package net

import (
	"bytes"
	"context"
	"io"
	"math/rand"
	"sync"
	"sync/atomic"

	"github.com/jbenet/goprocess"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/record"
	ctxutil "github.com/lthibault/util/ctx"
)

type neighborhood struct {
	vtx vertex

	gcCtr sync.WaitGroup
	lease chan leaseRequest
	evict chan peer.ID
}

func newNeighborhood() *neighborhood {
	return &neighborhood{
		lease: make(chan leaseRequest, 1),
		evict: make(chan peer.ID, 1),
	}
}

func (n *neighborhood) SetUp(ch chan<- EvtState) goprocess.ProcessFunc {
	return func(p goprocess.Process) {
		e := n.newEmitter(p, ch)
		defer close(ch)

		for {
			select {
			case <-p.Closing():
				return

			case req := <-n.lease:
				n.handleLease(e, req)

			case id := <-n.evict:
				n.handleEvict(e, id)
			}
		}
	}
}

func (n *neighborhood) TearDown(state io.Closer) goprocess.TeardownFunc {
	return func() error {
		n.gcCtr.Wait()
		close(n.lease)
		close(n.evict)
		return state.Close()
	}
}

// func (n *neighborhood) Neighbors() peer.IDSlice {
// 	vtx := n.vtx.Load()
// 	ns := make(peer.IDSlice, 0, len(vtx))
// 	for id := range vtx {
// 		ns = append(ns, id)
// 	}
// 	return ns
// }

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

func (n *neighborhood) Lease(ctx context.Context, s network.Stream) (context.Context, bool) {
	req := newLeaseRequest(s)

	select {
	case <-ctx.Done():
		return nil, false
	case n.lease <- req:
		return req.Wait(ctx)
	}
}

func (n *neighborhood) Evict(ctx context.Context, id peer.ID) {
	select {
	case <-ctx.Done():
	default:
		select {
		case <-ctx.Done():
		case n.evict <- id:
		}
	}
}

func (n *neighborhood) handleLease(e emitter, req leaseRequest) {
	es := n.vtx.Load()

	// duplicate edge?
	if _, ok := es[req.edge.ID()]; ok {
		req.fail()
		return
	}

	es = es.Copy()
	es[req.edge.ID()] = req.edge
	n.vtx.Store(es)

	req.succeed()
	e.Emit(req.edge.ID(), EventJoined)
}

func (n *neighborhood) handleEvict(e emitter, id peer.ID) {
	es := n.vtx.Load()
	if edge, ok := es[id]; ok {
		defer edge.Close()

		es = es.Copy()
		delete(es, id)
		n.vtx.Store(es)

		e.Emit(edge.ID(), EventLeft)
	}
}

type emitter func(peer.ID, Event)

func (n *neighborhood) newEmitter(p goprocess.Process, ch chan<- EvtState) emitter {
	return func(id peer.ID, ev Event) {
		select {
		case <-p.Closing():
		case ch <- EvtState{Peer: id, Event: ev, es: n.vtx.Load()}:
		}
	}
}

func (emit emitter) Emit(id peer.ID, ev Event) { emit(id, ev) }

func (n *neighborhood) Handle(ctx context.Context, log Logger, h host.Host, s network.Stream) {
	var ss sampleStep
	if err := ss.RecvPayload(s); err != nil {
		log.WithError(err).
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

	ctx, cancel := context.WithDeadline(ctx, ss.Deadline())
	defer cancel()

	if err := ss.Next(s, h, peer).Step(ctx); err != nil {
		log.WithError(err).
			Debug("error encountered during random walk (next step)")
	}
}

func (n *neighborhood) Peers() []*peer.PeerRecord {
	es := n.vtx.Load()
	peers := make([]*peer.PeerRecord, len(es))

	i := 0
	for p := range es {
		peers[i] = peer.NewPeerRecord()
		peers[i].PeerID = p
		// TODO: Addrs is filled outside this function, by the caller
		i++
	}
	return peers
}

type vertex struct {
	r     *rand.Rand
	value atomic.Value
}

func (vtx *vertex) Random() (edge, bool) {
	es := vtx.Load()
	if len(es) == 0 {
		return edge{}, false
	}

	slice := make([]edge, 0, len(es))

	for _, e := range es {
		select {
		case <-e.Context().Done():
		default:
			slice = append(slice, e)
		}
	}

	vtx.r.Shuffle(len(slice), func(i, j int) {
		slice[i], slice[j] = slice[j], slice[i]
	})

	if len(slice) == 0 {
		return edge{}, false
	}
	return slice[0], true
}

func (vtx *vertex) Get(id peer.ID) (edge, bool) {
	if e, ok := vtx.Load()[id]; ok {
		select {
		case <-e.Context().Done():
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
	cq chan struct{}
	s  network.Stream
}

func (e edge) ID() peer.ID              { return e.s.Conn().RemotePeer() }
func (e edge) Context() context.Context { return ctxutil.FromChan(e.cq) }

func (e edge) Close() error {
	select {
	case <-e.cq:
	default:
		close(e.cq)
	}
	return e.s.Reset()
}

type edgeMap map[peer.ID]edge

func (m edgeMap) Copy() edgeMap {
	new := make(edgeMap, len(m))
	for id, e := range m {
		select {
		case <-e.Context().Done():
		default:
			new[id] = e
		}
	}
	return new
}

func (m edgeMap) Slice() peer.IDSlice {
	ns := make(peer.IDSlice, 0, len(m))
	for id, e := range m {
		select {
		case <-e.Context().Done():
		default:
			ns = append(ns, id)
		}
	}
	return ns
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
	edge edge
	done chan context.Context
}

func newLeaseRequest(s network.Stream) leaseRequest {
	return leaseRequest{
		edge: edge{s: s, cq: make(chan struct{})},
		done: make(chan context.Context, 1),
	}
}

func (req leaseRequest) Wait(ctx context.Context) (context.Context, bool) {
	select {
	case <-ctx.Done():
		return nil, false
	case ctx, ok := <-req.done:
		return ctx, ok
	}
}

func (req leaseRequest) succeed() {
	go func() {
		req.done <- req.edge.Context()
		defer close(req.edge.cq)

		// close the edge's done channel when the stream terminates
		_, _ = io.Copy(io.Discard, req.edge.s)
	}()
}

func (req leaseRequest) fail() {
	close(req.done)
	close(req.edge.cq)
}
