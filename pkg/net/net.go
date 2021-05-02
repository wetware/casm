// Package net implements an overlay network
package net

import (
	"context"
	"github.com/jbenet/goprocess"
	"github.com/libp2p/go-eventbus"
	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	protoutil "github.com/wetware/casm/pkg/util/proto"
	"time"

	ctxutil "github.com/lthibault/util/ctx"
	syncutil "github.com/lthibault/util/sync"
)

const tag = "casm.net/neighborhood"

type Overlay struct {
	log Logger

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
	o.h.SetStreamHandlerMatch(sampleProto, o.matchSample, o.handleSample)
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

	for {
		time.Sleep(500 * time.Millisecond) // TODO: decide optimal tick for updating neighbors
		peer, err := o.n.RandPeer()
		if err != nil { // TODO: decide what to do errors
			continue
		}
		peers, err := gossip{
			o.n, o.h, o.h, peer, protoutil.Join(o.proto, "gossip"),
		}.PushPull(o.ctx())
		if err != nil {
			continue
		}
		err = o.setNeighborhood(o.ctx(), peers)
		if err != nil {
			continue
		}
	}
}

func (o *Overlay) setNeighborhood(ctx context.Context, peers []*peer.PeerRecord) error {
	// get new nodes
	oldPeers := o.n.Peers()
	newPeers := make(StaticAddrs, len(peers))
	for _, p := range peers {
		if !contains(oldPeers, p) {
			newPeers = append(newPeers, peer.AddrInfo{p.PeerID, p.Addrs})
		}
	}
	// get evict nodes
	evictPeers := []*peer.PeerRecord{}
	for _, p := range oldPeers {
		if !contains(peers, p) {
			evictPeers = append(evictPeers, p)
		}
	}
	// join new nodes
	err := o.Join(ctx, newPeers)
	if err != nil {
		return err
	}
	// evict old unused nodes
	for _, p := range evictPeers {
		o.n.Evict(ctx, p.PeerID)
	}
	return nil
}

func contains(peers []*peer.PeerRecord, rec *peer.PeerRecord) bool {
	for _, p := range peers {
		if p.PeerID == rec.PeerID {
			return true
		}
	}
	return false
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
	for _, id := range []protocol.ID{joinProto, sampleProto} {
		o.h.RemoveStreamHandler(id)
	}

	o.log.WithField("proto", o.proto).Debug("unregistered protocol handlers")

	return o.proc.Close()
}

// Process for the overlay network.
func (o *Overlay) Process() goprocess.Process { return o.proc }

// Stat returns the current state of the overlay.  The returned slice
// contains the peer IDs of all immediate neighbors.
func (o *Overlay) Stat() peer.IDSlice { return o.n.vtx.Load().Slice() }

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

// FindPeers in the overlay by performing a random walk.  The ns value
// is ignored.  The discovery.TTL option is not supported and returns
// an error.
//
// FindPeers satisfies discovery.Discoverer, making it possible to pass
// an overlay to its own Join method.
func (o *Overlay) FindPeers(ctx context.Context, ns string, opt ...discovery.Option) (<-chan peer.AddrInfo, error) {
	var options sampleOpts
	if err := options.Apply(opt...); err != nil {
		return nil, err
	}

	var (
		j  syncutil.Join
		ch = make(chan peer.AddrInfo, options.Limit)
	)

	for i := 0; i < options.Breadth(); i++ {
		j.Go(o.sample(ctx, ch, options.Depth()))
	}

	go func() {
		defer close(ch)

		if err := j.Wait(); err != nil {
			o.log.WithError(err).Debug("error encountered while sampling")
		}
	}()

	return ch, nil
}

func (o *Overlay) sample(ctx context.Context, ch chan<- peer.AddrInfo, d uint8) func() error {
	return func() error {
		peer, err := o.n.RandPeer()
		if err != nil {
			return err
		}

		return randWalk{
			peer:  peer,
			depth: d,
			d:     deliveryChan(ch),
			sd:    o.h,
			proto: protoutil.Join(o.proto, "sample"),
		}.Step(ctx)
	}
}

func (o *Overlay) ctx() context.Context {
	return ctxutil.FromChan(o.proc.Closing())
}

func (o *Overlay) handleJoin(s network.Stream) {
	defer s.Close()

	if ctx, ok := o.n.Lease(o.ctx(), s); ok {
		peer := s.Conn().RemotePeer()
		defer o.n.Evict(o.ctx(), peer)

		o.h.ConnManager().Protect(peer, tag)
		defer o.h.ConnManager().Unprotect(peer, tag)

		<-ctx.Done()
	}
}

func (o *Overlay) handleSample(s network.Stream) {
	defer s.Close()

	o.n.Handle(o.ctx(), o.log.WithStream(s), o.h, s)
}

func (o *Overlay) handleGossip(s network.Stream) {
	defer s.Close()

	g := gossip{
		o.n, o.h, o.h, s.Conn().RemotePeer(), protoutil.Join(o.proto, "gossip"),
	}
	peers, err := g.Pull(o.ctx(), s)
	if err != nil {
		return
	}
	err = g.Push(o.ctx(), s)
	if err != nil {
		return
	}
	err = o.setNeighborhood(o.ctx(), peers)
	if err != nil {
		return
	}
}

type deliveryChan chan<- peer.AddrInfo

func (ch deliveryChan) Deliver(ctx context.Context, rec *peer.PeerRecord) error {
	select {
	case ch <- peer.AddrInfo{ID: rec.PeerID, Addrs: rec.Addrs}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
