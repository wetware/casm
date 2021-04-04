// Package net implements an overlay network
package net

import (
	"context"

	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"

	syncutil "github.com/lthibault/util/sync"
)

const tag = "casm.net/neighborhood"

type Overlay struct {
	log Logger

	h host.Host

	ns string
	n  neighborhood
}

// New network overlay
func New(h host.Host, opt ...Option) (*Overlay, error) {
	var (
		err error
		o   = &Overlay{h: h}
	)
	for _, option := range withDefaults(opt) {
		option(o)
	}

	if err = o.n.init(o.log, h); err != nil {
		return nil, err
	}

	o.h.Network().Notify((*notifiee)(o))
	o.h.SetStreamHandler(ProtocolID, o.handle)
	o.h.SetStreamHandler(SampleID, o.n.Handle)

	return o, nil
}

// Close the overlay network, freeing all resources.  Does not close the
// underlying host.
func (o *Overlay) Close() error {
	o.h.RemoveStreamHandler(ProtocolID)
	o.h.Network().StopNotify((*notifiee)(o))

	o.log.WithField("proto", ProtocolID).Debug("unregistered stream handlers")

	return o.n.Close()
}

// Loggable representation of the neighborhood
func (o *Overlay) Loggable() map[string]interface{} {
	return map[string]interface{}{
		"type": "casm.net.overlay",
		"id":   o.h.ID(),
		"ns":   o.ns,
	}
}

// Stat returns the current state of the overlay.  The returned slice
// contains the peer IDs of all immediate neighbors.
func (o *Overlay) Stat() peer.IDSlice { return o.n.Neighbors() }

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
		if err := o.h.Connect(ctx, info); err != nil {
			return err
		}

		s, err := o.h.NewStream(ctx, info.ID, ProtocolID)
		if err != nil {
			return err
		}

		ctx, err = o.n.Lease(ctx, s)
		switch err {
		case nil:
			go o.waitAndEvict(ctx, s.Conn().RemotePeer())
		case errEdgeExists:
		default:
			return err
		}

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
		}.Step(ctx)
	}
}

func (o *Overlay) handle(s network.Stream) {
	ctx, err := o.n.Lease(o.n.Context(), s)
	if err != nil {
		o.log.WithStream(s).
			WithError(err).
			Debug("error encountered while processing lease request")
		return
	}

	o.waitAndEvict(ctx, s.Conn().RemotePeer())
}

func (o *Overlay) waitAndEvict(ctx context.Context, id peer.ID) {
	select {
	case <-o.n.Context().Done():
	case <-ctx.Done():
		o.n.Evict(id)
	}
}

type notifiee Overlay

const incr, decr = 1, -1

func (*notifiee) Listen(_ network.Network, _ multiaddr.Multiaddr) {}

func (*notifiee) ListenClose(_ network.Network, _ multiaddr.Multiaddr) {}

func (*notifiee) Connected(_ network.Network, c network.Conn) {}
func (n *notifiee) Disconnected(_ network.Network, c network.Conn) {
	n.n.Evict(c.RemotePeer())
}

func (n *notifiee) OpenedStream(_ network.Network, s network.Stream) {
	if s.Protocol() != ProtocolID {
		n.UpsertTag(s.Conn().RemotePeer(), incr)
	}
}

func (n *notifiee) ClosedStream(_ network.Network, s network.Stream) {
	if s.Protocol() != ProtocolID {
		n.UpsertTag(s.Conn().RemotePeer(), decr)
	}
}

func (n *notifiee) UpsertTag(id peer.ID, sign int) {
	n.h.ConnManager().UpsertTag(id, tag, func(i int) int { return i + sign })
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
