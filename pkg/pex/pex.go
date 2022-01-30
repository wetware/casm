package pex

import (
	"context"

	"io"
	"time"

	"capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/rpc"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/protocol"

	"github.com/lthibault/log"
	syncutil "github.com/lthibault/util/sync"
	casm "github.com/wetware/casm/pkg"
	protoutil "github.com/wetware/casm/pkg/util/proto"
)

const (
	mtu     = 2048             // maximum transmission unit => max capnp message size
	timeout = time.Second * 15 // Push-pull timeout
)

type StreamHandler interface {
	SetStreamHandlerMatch(protocol.ID, func(string) bool, network.StreamHandler)
	RemoveStreamHandler(pid protocol.ID)
}

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
	store GossipStore
	mint  Mint
}

// New peer exchange.
func New(ctx context.Context, m Mint, gs GossipStore, opt ...Option) (*PeerExchange, error) {
	var px = PeerExchange{
		store: gs,
		mint:  m,
	}
	for _, option := range withDefaults(opt) {
		option(&px)
	}

	return &px, nil
}

func (px *PeerExchange) String() string { return px.store.String() }

func (px *PeerExchange) Loggable() map[string]interface{} {
	return px.store.Loggable()
}

func (px *PeerExchange) RegisterRPC(log log.Logger, h StreamHandler) {
	var (
		match       = casm.NewMatcher(px.String())
		matchPacked = match.Then(protoutil.Exactly("packed"))
	)

	h.SetStreamHandlerMatch(
		casm.Subprotocol(px.String()),
		match,
		px.newHandler(log, rpc.NewStreamTransport))

	h.SetStreamHandlerMatch(
		casm.Subprotocol(px.String(), "packed"),
		matchPacked,
		px.newHandler(log, rpc.NewPackedStreamTransport))
}

func (px *PeerExchange) UnregisterRPC(h StreamHandler) {
	h.RemoveStreamHandler(casm.Subprotocol(px.String()))
	h.RemoveStreamHandler(casm.Subprotocol(px.String(), "packed"))
}

func (px *PeerExchange) newHandler(log log.Logger, f transportFactory) network.StreamHandler {
	return func(s network.Stream) {
		slog := log.
			With(streamFields(s)).
			With(px.mint).
			With(px)
		defer s.Close()

		ctx, cancel := context.WithTimeout(context.TODO(), timeout)
		defer cancel()

		log.Debug("handler started")
		defer func() { log.Debug("handler finished") }()

		if err := px.pushpull(ctx, s); err != nil {
			slog.WithError(err).Debug("peer exchange failed")
		}
	}
}

func (px *PeerExchange) pushpull(ctx context.Context, s network.Stream) error {
	var (
		j      syncutil.Join
		t, _   = ctx.Deadline()
		remote gossipSlice
	)

	if err := s.SetDeadline(t); err != nil {
		return err
	}

	recs, err := px.store.LoadRecords()
	if err != nil {
		return err
	}

	recs = recs.Bind(sorted())
	oldest := recs.Bind(px.store.tail())

	local := append(
		recs.
			Bind(head(len(recs)-len(oldest))).
			Bind(shuffled()),
		oldest...)

	// push
	j.Go(func() error {
		defer s.CloseWrite()

		buffer := local.
			Bind(isNot(s.Conn().RemotePeer())).
			Bind(px.store.head()).
			Bind(appendLocal(px.mint))

		enc := capnp.NewPackedEncoder(s)
		for _, g := range buffer {
			if err = enc.Encode(g.Message()); err != nil {
				break
			}
		}

		return err
	})

	// pull
	j.Go(func() error {
		defer s.CloseRead()

		r := io.LimitReader(s, px.store.mtu())

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
		return nil
	})

	if err = j.Wait(); err == nil {
		err = px.store.MergeAndStore(
			px.mint.Load().Record.PeerID,
			local,
			remote)
	}

	return err
}

type transportFactory func(io.ReadWriteCloser) rpc.Transport

func (f transportFactory) NewTransport(rwc io.ReadWriteCloser) rpc.Transport { return f(rwc) }

func streamFields(s network.Stream) log.F {
	return log.F{
		"peer":   s.Conn().RemotePeer(),
		"conn":   s.Conn().ID(),
		"proto":  s.Protocol(),
		"stream": s.ID(),
	}
}

/*

 ...

*/

// // Bootstrap a namespace by performing an initial gossip round with a known peer.
// func (px *PeerExchange) Bootstrap(ctx context.Context, ns string, peer peer.AddrInfo) error {
// 	if err := px.h.Connect(ctx, peer); err != nil {
// 		return err
// 	}

// 	s, err := px.h.NewStream(ctx, peer.ID, proto(ns))
// 	if err != nil {
// 		return streamError{Peer: peer.ID, error: err}
// 	}
// 	defer s.Close()

// 	return px.pushpull(ctx, px.namespace(ns), s)
// }

// func (px *PeerExchange) Advertise(ctx context.Context, ns string, opt ...discovery.Option) (time.Duration, error) {
// 	opts, err := px.options(ns, opt)
// 	if err != nil {
// 		return 0, err
// 	}

// 	ctx, cancel := context.WithTimeout(ctx, timeout)
// 	defer cancel()

// 	px.trackNamespace(ns, opts.Ttl)

// 	// try the gossip cache first; if it's empty (or if we fail to connect
// 	// to the peers within), fall back on the discovery service.
// 	if err = px.gossipCache(ctx, ns); err != nil && px.d != nil {
// 		err = px.gossipDiscover(ctx, ns)
// 	}

// 	return opts.Ttl, err
// }

// func (px *PeerExchange) FindPeers(ctx context.Context, ns string, opt ...discovery.Option) (<-chan peer.AddrInfo, error) {
// 	opts, err := px.options(ns, opt)
// 	if err != nil {
// 		return nil, err
// 	}

// 	view, err := px.namespace(ns).View()
// 	if err != nil {
// 		return nil, err
// 	}

// 	out := make(chan peer.AddrInfo, len(view))
// 	defer close(out)

// 	for _, info := range limit(opts, view) {
// 		out <- info
// 	}

// 	return out, nil
// }

// func (px *PeerExchange) options(ns string, opt []discovery.Option) (opts *discovery.Options, err error) {
// 	opts = &discovery.Options{Limit: px.newGossip(ns).C}
// 	if err = opts.Apply(opt...); err == nil && opts.Ttl == 0 {
// 		opts.Ttl = px.newTick(ns)
// 	}

// 	return
// }

// func (px *PeerExchange) gossipCache(ctx context.Context, ns string) error {
// 	view, err := px.namespace(ns).View()
// 	if err != nil {
// 		return err
// 	}

// 	if len(view) == 0 {
// 		return errors.New("orphaned host")
// 	}

// 	// local host should not be in view

// 	for _, info := range view {
// 		if err = px.Bootstrap(ctx, ns, info); err != nil {
// 			if se, ok := err.(streamError); ok {
// 				px.log.With(se).Debug("gossip error")
// 				continue
// 			}
// 		}

// 		break
// 	}

// 	return err
// }

// func (px *PeerExchange) gossipDiscover(ctx context.Context, ns string) error {
// 	ps, err := px.d.FindPeers(ctx, ns)
// 	if err != nil {
// 		return err
// 	}

// 	for info := range ps {
// 		if info.ID == px.h.ID() {
// 			continue // don't dial self
// 		}

// 		if err = px.Bootstrap(ctx, ns, info); err != nil {
// 			if se, ok := err.(streamError); ok {
// 				px.log.With(se).Debug("gossip error")
// 				continue
// 			}
// 		}
// 		break
// 	}

// 	return err
// }

// type tracker struct {
// }

// func (px *PeerExchange) matcher() (func(s string) bool, error) {
// 	versionOK, err := helpers.MultistreamSemverMatcher(Proto)
// 	return func(s string) bool {
// 		return versionOK(path.Dir(s))
// 	}, err
// }

// func (px *PeerExchange) setLocalRecord(e *record.Envelope) error {
// 	g, err := NewGossipRecord(e)
// 	if err == nil {
// 		px.self.Store(g)
// 	}
// 	return err
// }

// func proto(ns string) protocol.ID {
// 	return protoutil.AppendStrings(Proto, ns)
// }

// func limit(opts *discovery.Options, is []peer.AddrInfo) []peer.AddrInfo {
// 	if opts.Limit < len(is) {
// 		is = is[:opts.Limit]
// 	}

// 	return is
// }

// func (px *PeerExchange) namespace(ns string) namespace {
// 	// function for abstracting of internal management of the namespace
// 	return px.getOrCreateNamespace(ns)
// }

// func (px *PeerExchange) trackNamespace(ns string, ttl time.Duration) {
// 	for {
// 		select {
// 		case px.getOrCreateNamespace(ns).ttl <- ttl:
// 			return
// 		case <-px.getOrCreateNamespace(ns).ctx.Done():
// 			// If namespace context is Done, the loop continues
// 			// until a new namespace is created and the TTL is set
// 		}
// 	}
// }

// func (px *PeerExchange) getOrCreateNamespace(nss string) namespace {
// 	px.mu.Lock()
// 	defer px.mu.Unlock()

// 	if ns, ok := px.ns[nss]; ok {
// 		return ns
// 	}

// 	ctx, cancel := context.WithCancel(context.Background())
// 	ns := namespace{
// 		nss:    nss,
// 		prefix: px.prefix.ChildString(nss),
// 		id:     px.h.ID(),
// 		gossip: px.newGossip(nss),
// 		e:      px.e,
// 		ds:     px.newStore(nss),

// 		ttl:    make(chan time.Duration),
// 		ctx:    ctx,
// 		cancel: cancel,
// 	}
// 	px.ns[nss] = ns
// 	// Namespace is reomved after 15 seconds,
// 	// unless the user requests for a longer tracking TTL
// 	go px.removeNamespace(ns, 15*time.Second)
// 	return ns
// }

// func (px *PeerExchange) removeNamespace(ns namespace, ttl time.Duration) {
// 	timer := time.NewTimer(ttl)
// 	defer timer.Stop()

// 	for {
// 		select {
// 		case <-timer.C:
// 			px.mu.Lock()
// 			defer px.mu.Unlock()
// 			delete(px.ns, ns.nss)
// 			ns.cancel()
// 			return
// 		case ttl := <-ns.ttl:
// 			timer.Reset(ttl)
// 		}
// 	}
// }

// /*
//  * Set-up functions
//  */

// // suppliedComponents is  needed in order for fx to correctly handle
// // the 'host.Host' interface.  Simply relying on 'fx.Supply' will
// // use the type of the underlying host implementation, which may
// // vary.
// type suppliedComponents struct {
// 	fx.Out

// 	Ctx      context.Context
// 	ID       peer.ID
// 	Bus      event.Bus
// 	Host     host.Host
// 	PrivKey  crypto.PrivKey
// 	CertBook ps.CertifiedAddrBook
// }

// // Supply is used to pass interfaces to the dependency-injection framework.
// // This is used in cases where fx's reflection erroneously uses the value's
// // concrete type instead of its interface type.
// //
// // As a matter of convenience, we also provide commonly-used components
// // of 'Host' directly.
// func supply(ctx context.Context, h host.Host) func(fx.Lifecycle) (suppliedComponents, error) {
// 	return func(lx fx.Lifecycle) (cs suppliedComponents, err error) {
// 		cb, ok := ps.GetCertifiedAddrBook(h.Peerstore())
// 		if !ok {
// 			return suppliedComponents{}, errNoSignedAddrs
// 		}

// 		// cancel the context when the PeerExchange is closed.
// 		ctx, cancel := context.WithCancel(ctx)
// 		lx.Append(fx.Hook{
// 			OnStop: func(context.Context) error {
// 				cancel()
// 				return nil
// 			},
// 		})

// 		return suppliedComponents{
// 			Ctx:      ctx,
// 			Host:     h,
// 			ID:       h.ID(),
// 			Bus:      h.EventBus(),
// 			PrivKey:  h.Peerstore().PrivKey(h.ID()),
// 			CertBook: cb,
// 		}, nil
// 	}
// }

// func newConfig(id peer.ID, opt []Option) (c Config) {
// 	c.Apply(opt)
// 	c.Log = c.Log.WithField("id", id)
// 	return
// }

// type pexParams struct {
// 	fx.In

// 	Ctx         context.Context
// 	Log         log.Logger
// 	Host        host.Host
// 	NewGossip   func(ns string) Gossip
// 	TickFactory func(ns string) time.Duration
// 	NewStore    func(ns string) ds.Batching
// 	Disc        discovery.Discovery
// }

// func (p pexParams) Prefix() ds.Key {
// 	return ds.NewKey(p.Host.ID().String())
// }

// func newPeerExchange(p pexParams) (*PeerExchange, error) {
// 	e, err := p.Host.EventBus().Emitter(new(EvtViewUpdated))
// 	return &PeerExchange{
// 		ctx:       p.Ctx,
// 		log:       p.Log,
// 		newGossip: p.NewGossip,
// 		h:         p.Host,
// 		d:         p.Disc,
// 		newTick:   p.TickFactory,
// 		newStore:  p.NewStore,
// 		ns:        make(map[string]namespace),
// 		prefix:    p.Prefix(),
// 		e:         e,
// 	}, err
// }

// func newEvents(bus event.Bus, lx fx.Lifecycle) (s event.Subscription, err error) {
// 	if s, err = bus.Subscribe(new(event.EvtLocalAddressesUpdated)); err == nil {
// 		lx.Append(closer(s))
// 	}

// 	return
// }

// type runParam struct {
// 	fx.In

// 	Log  log.Logger
// 	Host host.Host
// 	Sub  event.Subscription
// 	CAB  ps.CertifiedAddrBook
// 	PeX  *PeerExchange
// }

// func (p runParam) Go(f func(log.Logger, <-chan interface{}, *PeerExchange, ps.CertifiedAddrBook)) fx.Hook {
// 	return fx.Hook{
// 		OnStart: func(context.Context) error {
// 			go f(p.Log, p.Sub.Out(), p.PeX, p.CAB)
// 			return nil
// 		},
// 	}
// }

// // func run(p runParam, lx fx.Lifecycle) {
// // 	// // Block until the host's signed record has propagated.  Note
// // 	// // that 'EvtLocalAddressesUpdated' is a stateful subscription,
// // 	// // so we can be certain that this does not block indefinitely.
// // 	// lx.Append(fx.Hook{OnStart: func(ctx context.Context) error {
// // 	// 	select {
// // 	// 	case v, ok := <-p.Sub.Out():
// // 	// 		if ok {
// // 	// 			ev := v.(event.EvtLocalAddressesUpdated)
// // 	// 			return p.PeX.setLocalRecord(ev.SignedPeerRecord)
// // 	// 		}

// // 	// 	case <-ctx.Done():
// // 	// 		return ctx.Err()
// // 	// 	}

// // 	// 	return errors.New("host shutting down")
// // 	// }})

// // 	// // Once the local record is safely stored in the gossipStore,
// // 	// // we can begin processing events normally in the background.
// // 	// lx.Append(p.Go(func(log log.Logger, events <-chan interface{}, px *PeerExchange, c ps.CertifiedAddrBook) {
// // 	// 	for v := range events {
// // 	// 		ev := v.(event.EvtLocalAddressesUpdated)
// // 	// 		if err := px.setLocalRecord(ev.SignedPeerRecord); err != nil {
// // 	// 			log.WithError(err).
// // 	// 				Error("error updating local record")
// // 	// 		}
// // 	// 	}
// // 	// }))

// // 	// // Now that the exchange's state is consistent and all background tasks
// // 	// // have started, we can connect the stream handlers and begin gossipping.
// // 	// lx.Append(fx.Hook{
// // 	// 	OnStart: func(ctx context.Context) error {
// // 	// 		match, err := p.PeX.matcher()
// // 	// 		if err == nil {
// // 	// 			p.Host.SetStreamHandlerMatch(Proto, match, p.PeX.handler(ctx))
// // 	// 		}
// // 	// 		return err
// // 	// 	},
// // 	// 	OnStop: func(context.Context) error {
// // 	// 		p.Host.RemoveStreamHandler(Proto)
// // 	// 		return nil
// // 	// 	},
// // 	// })
// // }

// // func closer(c io.Closer) fx.Hook {
// // 	return fx.Hook{
// // 		OnStop: func(context.Context) error {
// // 			return c.Close()
// // 		},
// // 	}
// // }
