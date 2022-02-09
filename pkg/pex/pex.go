package pex

import (
	"context"
	"sync/atomic"

	"time"

	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/wetware/casm/pkg/boot"

	"github.com/lthibault/jitterbug/v2"
	"github.com/lthibault/log"
)

// PeerExchange is a collection of passive views of various p2p clusters.
//
// For each namespace that is joined, PeerExchange maintains a bounded set
// of random peer addresses via its gossip protocol.  Peers are not directly
// monitored for liveness, so the addresses returned from FindPeers may be
// stale.  However, the PeX gossip-protocol guarantees that stale addresses
// are eventually expunged.
//
// Note that this is behavior reflects a fundamental trade-off in the design
// of the PeX protocol.  PeX strives to maintain a passive view of clusters
// that can be used to repair partitions and reconnect orphaned peers.  As a
// result, it must not immediately expunge unreachable peers from its records,
// else this would cause partitions to rapidly "forget" about each other.
//
// For the above reasons, we encourage users NOT to tune PeX parameters, as
// these have been carefully selected to work in a broad range of applications
// and micro-optimizations are likely to be counterproductive.
type PeerExchange struct {
	ctx context.Context
	log log.Logger

	h         host.Host
	newParams func(string) GossipConfig

	t      time.Time
	as     map[string]advertiser
	thunks chan<- func()

	store rootStore
	disc  discover
}

func New(ctx context.Context, h host.Host, opt ...Option) (*PeerExchange, error) {
	sub, err := h.EventBus().Subscribe(new(event.EvtLocalAddressesUpdated))
	if err != nil {
		return nil, err
	}

	var thunks = make(chan func(), 1)

	var px = PeerExchange{
		ctx:    ctx,
		h:      h,
		thunks: thunks,
		as:     make(map[string]advertiser),
	}

	for _, option := range withDefaults(opt) {
		option(&px)
	}

	// Ensure the local record is stored before processing anything else.
	thunks <- func() {
		px.store.Consume((<-sub.Out()).(event.EvtLocalAddressesUpdated))
	}

	// Update
	go func() {
		defer sub.Close()

		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()

		for {
			select {
			case px.t = <-ticker.C:
				for ns, ad := range px.as {
					if ad.Expired(px.t) {
						ad.Gossiper.Stop()
						delete(px.as, ns)
					}
				}

			case f := <-thunks:
				f() // f is free to access shared fields

			case v := <-sub.Out():
				px.store.Consume(v.(event.EvtLocalAddressesUpdated))

			case <-ctx.Done():
				return
			}
		}
	}()

	return &px, nil
}

// Advertise triggers a gossip round for the specified namespace.
// The returned TTL is derived from the GossipParam instance
// associated with 'ns'. Any options passed to Advertise are ignored.
//
// The caller is responsible for calling Advertise with the same ns
// parameter as soon as the returned TTL has elapsed.  Failure to do
// so will cause the PeerExchange to eventually drop ns and to cease
// its participation in the namespace's gossip. A brief grace period
// is in effect, but SHOULD NOT be relied upon, and is therefore not
// documented.
func (px *PeerExchange) Advertise(ctx context.Context, ns string, _ ...discovery.Option) (time.Duration, error) {
	ch := make(chan *gossiper, 1) // TODO:  pool?

	select {
	case px.thunks <- func() {
		ad, ok := px.as[ns]
		if !ok {
			ad.Gossiper = px.newGossiper(ns)
			px.as[ns] = ad
			// XXX:  signal to the discovery service that we should bootstrap this namespace
		}

		ad.ResetTTL(px.t)
		ch <- ad.Gossiper
	}:

	case <-ctx.Done():
		return 0, ctx.Err()

	case <-px.ctx.Done():
		return 0, ErrClosed
	}

	g := <-ch

	// YOU ARE HERE
	//
	// 1. Try to load records from g
	// 2. If that fails, use px.disc to find a peer
	// 3. Then, open a stream 's' to that peer
	//

	if err := g.PushPull(ctx, s); err != nil {
		return 0, err
	}

	return jitterbug.
		Uniform{Min: g.config.Tick / 2}.
		Jitter(g.config.Tick), nil
}

func (px *PeerExchange) FindPeers(ctx context.Context, ns string, opt ...discovery.Option) (<-chan peer.AddrInfo, error) {
	var opts discovery.Options
	if err := opts.Apply(opt...); err != nil {
		return nil, err
	}

	var ch = make(chan *gossiper, 1) // TODO: pool
	select {
	case px.thunks <- func() { ch <- px.as[ns].Gossiper }:
	case <-ctx.Done():
		return nil, ctx.Err()

	case <-px.ctx.Done():
		return nil, ErrClosed
	}

	var g *gossiper
	select {
	case g = <-ch:
		if g == nil {
			return nil, ErrNotFound
		}

	case <-ctx.Done():
		return nil, ctx.Err()

	case <-px.ctx.Done():
		return nil, ErrClosed
	}

	v, err := g.store.LoadView()
	if err != nil {
		return nil, err
	}

	if len(v) == 0 {
		// XXX:  we should block until ctx expires, or until a gossip round completes
	}

	sa := make(boot.StaticAddrs, len(v))
	for i, r := range v.Bind(shuffled()) {
		sa[i].ID = r.PeerID
		sa[i].Addrs = r.Addrs
	}

	return sa.FindPeers(ctx, "")
}

type atomicRecord atomic.Value

func (rec *atomicRecord) Consume(e event.EvtLocalAddressesUpdated) {
	r, err := e.SignedPeerRecord.Record()
	if err != nil {
		panic(err)
	}

	(*atomic.Value)(rec).Store(r)
}

func (rec *atomicRecord) Record() *peer.PeerRecord {
	var v interface{}
	for v = (*atomic.Value)(rec).Load(); v == nil; {
		time.Sleep(time.Microsecond * 500)
	}
	return v.(*peer.PeerRecord)
}

type advertiser struct {
	Deadline time.Time
	Gossiper *gossiper
}

func (ad advertiser) Expired(t time.Time) bool {
	// Add a grace period equal to the timeout for a gossip round.
	// This prevents advertisers from being garbage-collected during
	// a gossip round.
	return t.After(ad.Deadline)
}

func (ad *advertiser) ResetTTL(t time.Time) {
	// Tick + Timeout serves as a grace period that prevents
	// an advertiser from being immediately dropped after the
	// TTL returned from Advertise has elapsed.  This is used
	// to prevent churn.  See Advertise for more details.
	d := ad.Gossiper.config.Tick + ad.Gossiper.config.Timeout
	ad.Deadline = t.Add(d)
}
