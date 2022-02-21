package pex

import (
	"context"
	"errors"
	"sync/atomic"

	"time"

	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/record"
	casm "github.com/wetware/casm/pkg"

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
	ctx    context.Context
	cancel context.CancelFunc
	log    log.Logger

	h         host.Host
	newParams func(ns string) GossipConfig

	t     time.Time
	as    map[string]advertiser
	thunk chan<- func()

	store rootStore
	disc  discover
}

func New(ctx context.Context, h host.Host, opt ...Option) (*PeerExchange, error) {
	if len(h.Addrs()) == 0 {
		return nil, errors.New("host not accepting connections")
	}

	sub, err := h.EventBus().Subscribe(new(event.EvtLocalAddressesUpdated))
	if err != nil {
		return nil, err
	}

	var thunks = make(chan func(), 1)

	ctx, cancel := context.WithCancel(ctx)
	var px = PeerExchange{
		ctx:    ctx,
		cancel: cancel,
		h:      h,
		thunk:  thunks,
		as:     make(map[string]advertiser),
	}

	for _, option := range withDefaults(opt) {
		option(&px)
	}

	// ensure the local record is stored before processing anything else.
	select {
	case ev := <-sub.Out():
		px.store.Consume((ev).(event.EvtLocalAddressesUpdated))
	case <-ctx.Done():
		return nil, ctx.Err()
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
						//px.disc.StopTracking(ns)
						delete(px.as, ns)
					}
				}

			case f := <-thunks:
				f() // f is free to access shared fields

			case v := <-sub.Out():
				px.store.Consume(v.(event.EvtLocalAddressesUpdated))

			case <-ctx.Done():
				px.close()
				return
			}
		}
	}()

	return &px, nil
}

func (px *PeerExchange) Close() {
	px.cancel()
}

func (px *PeerExchange) close() {
	for ns, ad := range px.as {
		ad.Gossiper.Stop()
		//px.disc.StopTracking(ns)
		delete(px.as, ns)
	}
}

func (px *PeerExchange) Bootstrap(ctx context.Context, ns string, peers ...peer.AddrInfo) error {
	g, err := px.getOrCreateGossiper(ctx, ns)
	if err != nil {
		return err
	}

	for _, info := range peers {
		if err = px.gossipRound(ctx, g, info); err == nil {
			return nil
		}
	}

	return errors.New("no peer found")
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
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	g, err := px.getOrCreateGossiper(ctx, ns)
	if err != nil {
		return 0, err
	}

	ttl := jitterbug.
		Uniform{Min: g.config.Tick / 2}.
		Jitter(g.config.Tick)

	// First, try cached peers
	cache, err := g.GetCachedPeers()
	if err != nil {
		return 0, err
	}

	for _, info := range cache {
		if err = px.gossipRound(ctx, g, info); err == nil {
			return ttl, nil
		}
	}

	// If cache is empty or all peers fail, fall back on boot service.
	peers, err := px.disc.Bootstrap(ctx, ns)
	if err != nil {
		return 0, err
	}

	for info := range peers {
		if err = px.gossipRound(ctx, g, info); err == nil {
			return ttl, nil
		}
		// TODO: log error?
	}

	return ttl, nil // no peer was found to advertise to (it may be the first node to join the network)
}

func (px *PeerExchange) FindPeers(ctx context.Context, ns string, opt ...discovery.Option) (<-chan peer.AddrInfo, error) {
	var opts discovery.Options
	if err := opts.Apply(opt...); err != nil {
		return nil, err
	}

	g, err := px.getGossiper(ctx, ns)
	if err != nil {
		return nil, err
	}

	// First, try cached peers
	cache, err := g.GetCachedPeers()
	if err != nil {
		return nil, err
	}

	cached := func(ctx context.Context) (<-chan peer.AddrInfo, error) {
		return cache.FindPeers(ctx, ns, opt...)
	}

	// If cache is empty or all peers fail, fall back on boot service.
	bootstrap := func(ctx context.Context) (<-chan peer.AddrInfo, error) {
		return px.disc.Bootstrap(ctx, ns)
	}

	out := make(chan peer.AddrInfo)
	go func() {
		defer close(out)

		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		for _, f := range []func(context.Context) (<-chan peer.AddrInfo, error){
			cached,
			bootstrap,
		} {
			peers, err := f(ctx)
			if err != nil {
				// TODO:  log
				continue
			}

			for info := range peers {
				select {
				case out <- info:
				case <-ctx.Done():
				case <-px.ctx.Done():
					return
				}
			}
		}
	}()

	return out, nil
}

func (px *PeerExchange) gossipRound(ctx context.Context, g *gossiper, info peer.AddrInfo) error {
	ctx, cancel := context.WithTimeout(ctx, g.config.Timeout)
	defer cancel()

	err := px.h.Connect(ctx, info)
	if err != nil {
		return err
	}

	s, err := px.h.NewStream(ctx, info.ID,
		casm.Subprotocol(g.String(), "packed"),
		casm.Subprotocol(g.String()))
	if err != nil {
		return err
	}

	defer s.Close()

	return g.PushPull(ctx, s)
}

type EvtPeersUpdated []*peer.PeerRecord

func (px *PeerExchange) getOrCreateGossiper(ctx context.Context, ns string) (*gossiper, error) {
	ch := make(chan *gossiper, 1) // TODO:  pool?
	e, err := px.h.EventBus().Emitter(new(EvtPeersUpdated))
	if err != nil {
		return nil, err
	}
	advertise := func() {
		ad, ok := px.as[ns]
		if !ok {
			ad.Gossiper = px.newGossiper(ns, e)
			px.as[ns] = ad
		}

		ad.ResetTTL(px.t)
		ch <- ad.Gossiper
	}

	select {
	case px.thunk <- advertise:
		return <-ch, nil

	case <-ctx.Done():
		return nil, ctx.Err()

	case <-px.ctx.Done():
		return nil, ErrClosed
	}
}

func (px *PeerExchange) getGossiper(ctx context.Context, ns string) (*gossiper, error) {
	var ch = make(chan *gossiper, 1) // TODO: pool

	select {
	case px.thunk <- func() {
		ch <- px.as[ns].Gossiper
	}:

	case <-ctx.Done():
		return nil, ctx.Err()

	case <-px.ctx.Done():
		return nil, ErrClosed
	}

	select {
	case g := <-ch:
		if g != nil {
			return g, nil
		}

	case <-ctx.Done():
		return nil, ctx.Err()

	case <-px.ctx.Done():
		return nil, ErrClosed
	}

	return nil, ErrNotFound
}

type atomicRecord struct {
	rec atomic.Value
	env atomic.Value
}

func (rec *atomicRecord) Consume(e event.EvtLocalAddressesUpdated) {
	r, err := e.SignedPeerRecord.Record()
	if err != nil {
		panic(err)
	}

	rec.env.Store(e.SignedPeerRecord)
	rec.rec.Store(r)
}

func (rec *atomicRecord) Record() *peer.PeerRecord {
	var v interface{}
	for v = rec.rec.Load(); v == nil; {
		time.Sleep(time.Microsecond * 500)
	}
	return v.(*peer.PeerRecord)
}

func (rec *atomicRecord) Envelope() *record.Envelope {
	var v interface{}
	for v = rec.env.Load(); v == nil; {
		time.Sleep(time.Microsecond * 500)
	}
	return v.(*record.Envelope)
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
