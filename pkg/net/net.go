// Package net implements an overlay network
package net

import (
	"context"
	"sort"
	"time"

	protoutil "github.com/wetware/casm/pkg/util/proto"

	"golang.org/x/sync/errgroup"

	"github.com/jbenet/goprocess"
	"github.com/libp2p/go-eventbus"
	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"

	"github.com/lthibault/jitterbug"
	ctxutil "github.com/lthibault/util/ctx"
	syncutil "github.com/lthibault/util/sync"
)

const tag = "casm.net/neighborhood"

type Overlay struct {
	log Logger
	r   *atomicRand

	h    host.Host
	proc goprocess.Process

	ns    string
	proto protocol.ID

	n *neighborhood
}

// New network overlay
func New(h host.Host, opt ...Option) (*Overlay, error) {
	var o = &Overlay{h: h, n: newNeighborhood()}
	for _, option := range withDefaults(opt) {
		option(o)
	}

	if err := o.init(); err != nil {
		return nil, err
	}

	return o, nil
}

func (o *Overlay) init() error {
	state, err := o.h.EventBus().Emitter(new(EvtState), eventbus.Stateful)
	if err != nil {
		return err
	}

	// Start the neighborhood process.  This is the overlay's root proc.
	ch := make(chan EvtState, 1)
	o.proc = o.h.Network().Process().Go(o.n.SetUp(ch))
	o.proc.SetTeardown(o.n.TearDown(state))

	go o.loop(ch, state)

	// The main loop is now running, so we can start accepting streams.
	o.h.SetStreamHandlerMatch(joinProto, o.matchJoin, o.handleJoin)
	o.h.SetStreamHandlerMatch(gossipProto, o.matchGossip, o.handleGossip)

	return nil
}

func (o *Overlay) loop(ch <-chan EvtState, state event.Emitter) {
	go func() {
		for ev := range ch {
			if err := state.Emit(ev); err != nil {
				o.log.With(ev).Error("failed to emit event")
			}
		}
	}()

	o.gossipLoop()
}

func (o *Overlay) gossipLoop() {
	bo := newBackoff(time.Millisecond*500, time.Minute)
	ticker := jitterbug.New(time.Millisecond*500, bo)

	defer ticker.Stop()

	for {
		bo.SetSteadyState(o.n.MaxLen() <= o.n.Len())
		select {
		case <-o.proc.Closing():
			return
		case <-ticker.C:
			p, err := o.n.RandPeer() // err is non-nil iff neighborhood is empty
			if err != nil {
				continue
			}
			s, err := o.h.NewStream(o.ctx(), p, protoutil.Join(o.proto, "gossip"))
			if err != nil {
				o.log.WithError(err).Error("opening of gossiper stream failed")
				continue
			}

			recs, err := gossiper{
				o.n, o.h, s, o.r,
			}.PushPull(o.ctx())
			if err != nil {
				o.log.WithError(err).Error("remote exchange failed")
				continue
			}
			err = o.setNeighborhood(o.ctx(), recs)
			if err != nil {
				o.log.WithError(err).Error("neighborhood update went wrong")
				continue
			}
			bo.Reset()
		}
	}
}

func (o *Overlay) setNeighborhood(ctx context.Context, recs recordSlice) error {
	oldRecs := o.n.Records()
	leasePeers := make(StaticAddrs, 0, len(recs))
	for _, rec := range recs.subtract(oldRecs) {
		leasePeers = append(leasePeers, peer.AddrInfo{ID: rec.PeerID, Addrs: rec.Addrs})
	}
	err := o.Join(ctx, leasePeers)
	if err != nil {
		return err
	}

	evictPeers := oldRecs.subtract(recs)
	for _, rec := range evictPeers {
		o.n.Evict(ctx, rec.PeerID)
	}
	return nil
}

// Loggable representation of the neighborhood
func (o *Overlay) Loggable() map[string]interface{} {
	return map[string]interface{}{
		"type": "casm.net.overlay",
		"id":   o.h.ID(),
		"ns":   o.ns,
	}
}

// Close the overlay network, freeing all resources.  Does not close the
// underlying host.
func (o *Overlay) Close() error {
	for _, id := range []protocol.ID{joinProto, gossipProto} {
		o.h.RemoveStreamHandler(id)
	}

	o.log.WithField("proto", o.proto).Debug("unregistered protocol handlers")

	return o.proc.Close()
}

// Process for the overlay network.
func (o *Overlay) Process() goprocess.Process { return o.proc }

// Stat returns the current state of the overlay. The returned slice
// contains the peer IDs of all immediate neighbors and the namespace.
func (o *Overlay) Stat() Status {
	return Status{peers: o.n.vtx.Load().Slice()}
}

// Host returns the underlying libp2p host.
func (o *Overlay) Host() host.Host { return o.h }

// Namespace of the overlay network.
func (o *Overlay) Namespace() string { return o.ns }

// Join the overlay network using the provided discovery service.
func (o *Overlay) Join(ctx context.Context, d discovery.Discoverer, opt ...discovery.Option) error {
	peers, err := d.FindPeers(ctx, o.ns, opt...)
	if err != nil {
		return err
	}

	var any syncutil.Any
	for info := range peers {
		any.Go(o.join(ctx, info))
	}

	if err = any.Wait(); err != nil {
		return JoinError{Report: ErrNoPeers, Cause: err}
	}

	return nil
}

func (o *Overlay) join(ctx context.Context, info peer.AddrInfo) func() error {
	return func() error {
		// edge exists?
		if _, ok := o.n.vtx.Get(info.ID); ok {
			return nil
		}

		if err := o.h.Connect(ctx, info); err != nil {
			return err
		}

		s, err := o.h.NewStream(ctx, info.ID, o.proto)
		if err != nil {
			return err
		}

		go o.handleJoin(s)

		return nil
	}
}

func (o *Overlay) ctx() context.Context {
	return ctxutil.FromChan(o.proc.Closing())
}

func (o *Overlay) handleJoin(s network.Stream) {
	defer s.Close()
	rec := o.newRecordFromStream(s)
	neighbors := o.n.Records()
	if o.n.MaxLen() <= len(neighbors) {
		if peersAreNear(o.h.ID(), rec.PeerID) {
			sort.Sort(neighbors)
			o.n.Evict(o.ctx(), neighbors[len(neighbors)-1].PeerID)
		} else {
			o.n.Evict(o.ctx(), neighbors[o.r.Intn(len(neighbors))].PeerID)
		}
	}
	if ctx, ok := o.n.Lease(o.ctx(), s, rec); ok {
		p := s.Conn().RemotePeer()
		defer o.n.Evict(o.ctx(), p)

		o.h.ConnManager().Protect(p, tag)
		defer o.h.ConnManager().Unprotect(p, tag)

		<-ctx.Done()
	}
}

func (o *Overlay) newRecordFromStream(s network.Stream) *peer.PeerRecord {
	peerId := s.Conn().RemotePeer()
	return &peer.PeerRecord{PeerID: peerId, Addrs: o.h.Peerstore().Addrs(peerId), Seq: 1}
}

func (o *Overlay) handleGossip(s network.Stream) {
	defer s.Close()

	gr, ctx := errgroup.WithContext(o.ctx())

	g := gossiper{
		o.n, o.h, s, o.r,
	}
	var recs recordSlice

	gr.Go(func() (err error) {
		recs, err = g.pull(ctx)
		return
	})
	gr.Go(func() error {
		return g.push(ctx)
	})
	err := gr.Wait()
	if err != nil {
		return
	}
	err = o.setNeighborhood(o.ctx(), recs)
	if err != nil {
		return
	}
}

type Status struct {
	peers peer.IDSlice
}

func (s *Status) Peers() peer.IDSlice {
	return s.peers
}
