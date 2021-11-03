package pex

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"sync"
	"sync/atomic"
	"time"

	"capnproto.org/go/capnp/v3"
	ds "github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/helpers"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	ps "github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/libp2p/go-libp2p-core/record"
	"go.uber.org/fx"

	"github.com/lthibault/log"
	syncutil "github.com/lthibault/util/sync"
	protoutil "github.com/wetware/casm/pkg/util/proto"
)

const (
	Version               = "0.0.0"
	baseProto             = "/casm/pex"
	Proto     protocol.ID = baseProto + "/" + Version

	mtu = 2048 // maximum transmission unit => max capnp message size
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
	ctx  context.Context
	log  log.Logger
	tick time.Duration

	h host.Host
	d *discover

	mu sync.Mutex
	self   atomic.Value
	ds     ds.Batching
	prefix ds.Key
	k      int // cardinality of the passive view
	e      event.Emitter
	
	runtime interface{ Stop(context.Context) error }
}

// New peer exchange.
func New(ctx context.Context, h host.Host, opt ...Option) (px *PeerExchange, err error) {
	if err = ErrNoListenAddrs; len(h.Addrs()) > 0 {
		app := fx.New(fx.NopLogger,
			fx.Populate(&px),
			fx.Supply(opt),
			fx.Provide(
				newConfig,
				newEvents,
				newDiscover,
				newPeerExchange,
				supply(ctx, h)),
			fx.Invoke(run))

		if err = app.Start(ctx); err == nil {
			px.runtime = app
		}
	}

	return
}

func (px *PeerExchange) Loggable() map[string]interface{} {
	return map[string]interface{}{
		"id": px.h.ID(),
		"k":  px.k,
	}
}

func (px *PeerExchange) Close() error { return px.runtime.Stop(context.Background()) }

// Bootstrap a namespace by performing an initial gossip round with a known peer.
func (px *PeerExchange) Bootstrap(ctx context.Context, ns string, peer peer.AddrInfo) error {
	if err := px.h.Connect(ctx, peer); err != nil {
		return err
	}

	s, err := px.h.NewStream(ctx, peer.ID, proto(ns))
	if err != nil {
		return streamError{Peer: peer.ID, error: err}
	}

	return px.pushpull(ctx, px.namespace(ns), s)
}

func (px *PeerExchange) Advertise(ctx context.Context, ns string, opt ...discovery.Option) (time.Duration, error) {
	opts, err := px.options(opt)
	if err != nil {
		return 0, err
	}

	if err := px.d.Track(px.ctx, ns, opts.Ttl); err != nil {
		return 0, err
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// try the gossip cache first; if it's empty (or if we fail to connect
	// to the peers within), fall back on the discovery service.
	if err = px.gossipCache(ctx, ns); err != nil && px.d != nil {
		err = px.gossipDiscover(ctx, ns)
	}

	return opts.Ttl, err
}

func (px *PeerExchange) FindPeers(ctx context.Context, ns string, opt ...discovery.Option) (<-chan peer.AddrInfo, error) {
	opts, err := px.options(opt)
	if err != nil {
		return nil, err
	}

	view, err := px.namespace(ns).View()
	if err != nil {
		return nil, err
	}

	out := make(chan peer.AddrInfo, len(view))
	defer close(out)

	for _, info := range limit(opts, view) {
		out <- info
	}

	return out, nil
}

func (px *PeerExchange) options(opt []discovery.Option) (opts *discovery.Options, err error) {
	opts = &discovery.Options{Limit: px.k}
	if err = opts.Apply(opt...); err == nil && opts.Ttl == 0 {
		opts.Ttl = px.tick
	}

	return
}

func (px *PeerExchange) gossipCache(ctx context.Context, ns string) error {
	view, err := px.namespace(ns).View()
	if err != nil {
		return err
	}

	if len(view) == 0 {
		return errors.New("orphaned host")
	}

	// local host should not be in view
	for _, info := range view {
		if err = px.bootstrap(ctx, ns, info); err != nil {
			if se, ok := err.(streamError); ok {
				px.log.With(se).Debug("gossip error")
				continue
			}
		}

		break
	}

	return err
}

func (px *PeerExchange) gossipDiscover(ctx context.Context, ns string) error {
	ps, err := px.d.Discover(ctx, ns)
	if err != nil {
		return err
	}

	for info := range ps {
		if info.ID == px.h.ID() {
			continue // don't dial self
		}

		if err = px.bootstrap(ctx, ns, info); err != nil {
			if se, ok := err.(streamError); ok {
				px.log.With(se).Debug("gossip error")
				continue
			}
		}

		break
	}

	return err
}

// bootstrap calls Bootstrap() with a timeout context
func (px *PeerExchange) bootstrap(ctx context.Context, ns string, info peer.AddrInfo) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second*15)
	defer cancel()

	return px.Bootstrap(ctx, ns, info)
}

func (px *PeerExchange) pushpull(ctx context.Context, n namespace, s network.Stream) error {
	fmt.Printf("%v: Waiting lock for conn with %v\n", n.id[:5], s.Conn().RemotePeer()[:5])
	px.mu.Lock()
	fmt.Printf("%v: Acquired lock for conn with %v\n", n.id[:5], s.Conn().RemotePeer()[:5])
	defer fmt.Printf("%v: Released lock for conn with %v\n", n.id[:5], s.Conn().RemotePeer()[:5])
	defer px.mu.Unlock()
	
	defer s.Close()

	var (
		t, _ = ctx.Deadline()
		j    syncutil.Join
	)

	if err := s.SetDeadline(t); err != nil {
		return err
	}

	// push
	j.Go(func() error {
		fmt.Printf("%v: Pushing to %v\n", n.id[:5], s.Conn().RemotePeer()[:5])
		defer fmt.Printf("%v: Pushed to %v\n", n.id[:5], s.Conn().RemotePeer()[:5])
		defer s.CloseWrite()

		gs, err := n.Records()
		if err != nil {
			return err
		}
		gs = append(
			gs.Bind(isNot(s.Conn().RemotePeer())), // save some bandwidth
			px.self.Load().(*GossipRecord))

		enc := capnp.NewPackedEncoder(s)
		for i, g := range gs {
			fmt.Printf("%v: Pushing %v/%v to %v\n", n.id[:5], i, len(gs)-1, s.Conn().RemotePeer()[:5])
			if err = enc.Encode(g.Message()); err != nil {
				break
			}
		}

		return err
	})

	// pull
	j.Go(func() error {
		defer s.CloseRead()

		var (
			remote gossipSlice
			r      = io.LimitReader(s, int64(px.k*mtu))
		)

		dec := capnp.NewPackedDecoder(r)
		dec.MaxMessageSize = mtu

		for {
			msg, err := dec.Decode()
			if err != nil {
				if err == io.EOF {
					break
				}
				return err
			}

			g := new(GossipRecord) // TODO(performance):  pool?
			if err = g.ReadMessage(msg); err != nil {
				return err
			}

			remote = append(remote, g)
		}

		return n.MergeAndStore(remote)
	})

	return j.Wait()
}

func (px *PeerExchange) namespace(ns string) namespace {
	return namespace{
		prefix: px.prefix.ChildString(ns),
		ds:     px.ds,
		id:     px.h.ID(),
		k:      px.k,
		e:      px.e,
	}
}

func (px *PeerExchange) matcher() (func(s string) bool, error) {
	versionOK, err := helpers.MultistreamSemverMatcher(Proto)
	return func(s string) bool {
		return versionOK(path.Dir(s))
	}, err
}

func (px *PeerExchange) handler(ctx context.Context) func(s network.Stream) {
	const timeout = time.Second * 15

	return func(s network.Stream) {
		defer s.Close()

		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		ns := path.Base(string(s.Protocol()))

		log := px.log.
			WithField("stream", s.ID()).
			WithField("proto", s.Protocol()).
			WithField("conn", s.Conn().ID()).
			WithField("peer", s.Conn().RemotePeer()).
			WithField("ns", ns).
			WithField("timeout", timeout)

		log.Debug("handler started")
		defer func() { log.Debug("handler finished") }()

		if err := px.pushpull(ctx, px.namespace(ns), s); err != nil {
			log = log.WithError(err)
		}
	}
}

func (px *PeerExchange) setLocalRecord(e *record.Envelope) error {
	g, err := NewGossipRecord(e)
	if err == nil {
		px.self.Store(g)
	}
	return err
}

func proto(ns string) protocol.ID {
	return protoutil.AppendStrings(Proto, ns)
}

func limit(opts *discovery.Options, is []peer.AddrInfo) []peer.AddrInfo {
	if opts.Limit < len(is) {
		is = is[:opts.Limit]
	}

	return is
}

/*
 * Set-up functions
 */

// suppliedComponents is  needed in order for fx to correctly handle
// the 'host.Host' interface.  Simply relying on 'fx.Supply' will
// use the type of the underlying host implementation, which may
// vary.
type suppliedComponents struct {
	fx.Out

	Ctx      context.Context
	ID       peer.ID
	Bus      event.Bus
	Host     host.Host
	PrivKey  crypto.PrivKey
	CertBook ps.CertifiedAddrBook
}

// Supply is used to pass interfaces to the dependency-injection framework.
// This is used in cases where fx's reflection erroneously uses the value's
// concrete type instead of its interface type.
//
// As a matter of convenience, we also provide commonly-used components
// of 'Host' directly.
func supply(ctx context.Context, h host.Host) func(fx.Lifecycle) (suppliedComponents, error) {
	return func(lx fx.Lifecycle) (cs suppliedComponents, err error) {
		cb, ok := ps.GetCertifiedAddrBook(h.Peerstore())
		if !ok {
			return suppliedComponents{}, errNoSignedAddrs
		}

		// cancel the context when the PeerExchange is closed.
		ctx, cancel := context.WithCancel(ctx)
		lx.Append(fx.Hook{
			OnStop: func(context.Context) error {
				cancel()
				return nil
			},
		})

		return suppliedComponents{
			Ctx:      ctx,
			Host:     h,
			ID:       h.ID(),
			Bus:      h.EventBus(),
			PrivKey:  h.Peerstore().PrivKey(h.ID()),
			CertBook: cb,
		}, nil
	}
}

func newConfig(id peer.ID, opt []Option) (c Config) {
	c.Apply(opt)
	c.Log = c.Log.WithField("id", id)
	return
}

type pexParams struct {
	fx.In

	Ctx   context.Context
	Log   log.Logger
	K     int
	Host  host.Host
	Tick  time.Duration
	Store ds.Batching
	Disc  *discover
}

func (p pexParams) Prefix() ds.Key {
	return ds.NewKey(p.Host.ID().String())
}

func newPeerExchange(p pexParams) (*PeerExchange, error) {
	e, err := p.Host.EventBus().Emitter(new(EvtViewUpdated))
	return &PeerExchange{
		ctx:    p.Ctx,
		log:    p.Log,
		k:      p.K,
		h:      p.Host,
		d:      p.Disc,
		tick:   p.Tick,
		ds:     p.Store,
		prefix: p.Prefix(),
		e:      e,
	}, err
}

func newEvents(bus event.Bus, lx fx.Lifecycle) (s event.Subscription, err error) {
	if s, err = bus.Subscribe(new(event.EvtLocalAddressesUpdated)); err == nil {
		lx.Append(closer(s))
	}

	return
}

type runParam struct {
	fx.In

	Log  log.Logger
	Host host.Host
	Sub  event.Subscription
	CAB  ps.CertifiedAddrBook
	PeX  *PeerExchange
}

func (p runParam) Go(f func(log.Logger, <-chan interface{}, *PeerExchange, ps.CertifiedAddrBook)) fx.Hook {
	return fx.Hook{
		OnStart: func(context.Context) error {
			go f(p.Log, p.Sub.Out(), p.PeX, p.CAB)
			return nil
		},
	}
}

func run(p runParam, lx fx.Lifecycle) {
	// Block until the host's signed record has propagated.  Note
	// that 'EvtLocalAddressesUpdated' is a stateful subscription,
	// so we can be certain that this does not block indefinitely.
	lx.Append(fx.Hook{OnStart: func(ctx context.Context) error {
		select {
		case v, ok := <-p.Sub.Out():
			if ok {
				ev := v.(event.EvtLocalAddressesUpdated)
				return p.PeX.setLocalRecord(ev.SignedPeerRecord)
			}

		case <-ctx.Done():
			return ctx.Err()
		}

		return errors.New("host shutting down")
	}})

	// Once the local record is safely stored in the gossipStore,
	// we can begin processing events normally in the background.
	lx.Append(p.Go(func(log log.Logger, events <-chan interface{}, px *PeerExchange, c ps.CertifiedAddrBook) {
		for v := range events {
			ev := v.(event.EvtLocalAddressesUpdated)
			if err := px.setLocalRecord(ev.SignedPeerRecord); err != nil {
				log.WithError(err).
					Error("error updating local record")
			}
		}
	}))

	// Now that the exchange's state is consistent and all background tasks
	// have started, we can connect the stream handlers and begin gossipping.
	lx.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			match, err := p.PeX.matcher()
			if err == nil {
				p.Host.SetStreamHandlerMatch(Proto, match, p.PeX.handler(ctx))
			}
			return err
		},
		OnStop: func(context.Context) error {
			p.Host.RemoveStreamHandler(Proto)
			return nil
		},
	})
}

func closer(c io.Closer) fx.Hook {
	return fx.Hook{
		OnStop: func(context.Context) error {
			return c.Close()
		},
	}
}
