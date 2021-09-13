package pex

import (
	"context"
	"errors"
	"io"
	"math/rand"
	"path"
	"time"

	"capnproto.org/go/capnp/v3"
	"github.com/libp2p/go-eventbus"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/helpers"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	ps "github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/libp2p/go-libp2p-core/protocol"
	"go.uber.org/fx"

	"github.com/lthibault/jitterbug/v2"
	"github.com/lthibault/log"
	syncutil "github.com/lthibault/util/sync"

	protoutil "github.com/wetware/casm/pkg/util/proto"
)

const (
	Version               = "0.0.0"
	baseProto protocol.ID = "/casm/pex"
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
	ns  string
	log log.Logger

	h  host.Host
	gs *gossipStore

	runtime fx.Shutdowner
}

// New peer exchange.
func New(h host.Host, opt ...Option) (px *PeerExchange, err error) {
	if err = ErrNoListenAddrs; len(h.Addrs()) > 0 {
		err = fx.New(fx.NopLogger,
			fx.Populate(&px),
			fx.Supply(opt),
			fx.Provide(
				newConfig,
				newEvents,
				newGossipStore,
				newPeerExchange,
				newHostComponents(h)),
			fx.Invoke(run)).
			Start(context.Background())
	}

	return
}

func (px *PeerExchange) String() string { return px.ns }
func (px *PeerExchange) Close() error   { return px.runtime.Shutdown() }

func (px *PeerExchange) Loggable() map[string]interface{} {
	return map[string]interface{}{
		"id":       px.h.ID(),
		"ns":       px.ns,
		"max_view": px.gs.MaxSize,
	}
}

// View is the set of peers contained in the passive view.
func (px *PeerExchange) View() ([]peer.AddrInfo, error) {
	gs, err := px.gs.Load()
	if err != nil {
		return nil, err
	}

	ps := make([]peer.AddrInfo, len(gs))
	for i, g := range gs {
		ps[i].ID = g.PeerID
		ps[i].Addrs = g.Addrs
	}

	return ps, nil
}

// Join the namespace using a bootstrap peer.
//
// Join blocks until the underlying host is listening on at least one network
// address.
func (px *PeerExchange) Join(ctx context.Context, boot peer.AddrInfo) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		s     network.Stream
		maybe breaker
	)

	for _, fn := range []func(){
		func() { maybe.Err = px.h.Connect(ctx, boot) },                     // connect to boot peer
		func() { s, maybe.Err = px.h.NewStream(ctx, boot.ID, px.proto()) }, // open pex stream
		func() { maybe.Err = px.pushpull(ctx, s) },                         // perform initial gossip round
	} {
		maybe.Do(fn)
	}

	return maybe.Err
}

func (px *PeerExchange) gossip(ctx context.Context) error {
	gs, err := px.gs.Load()
	if err != nil {
		return err
	}

	rand.Shuffle(len(gs), func(i, j int) {
		gs[i], gs[j] = gs[j], gs[i]
	})

	for _, g := range gs {
		err := px.gossipOne(ctx, g.PeerID)
		if se, ok := err.(streamError); ok {
			px.log.With(se).Debug("unable to connect")
			continue
		}

		return err
	}

	// we get here either if len(peers) == 0, or if all peers are unreachable.
	return errors.New("orphaned host")
}

func (px *PeerExchange) gossipOne(ctx context.Context, id peer.ID) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second*15)
	defer cancel()

	s, err := px.h.NewStream(ctx, id, px.proto())
	if err != nil {
		return streamError{Peer: id, error: err}
	}

	return px.pushpull(ctx, s)
}

func (px *PeerExchange) proto() protocol.ID {
	return protoutil.AppendStrings(Proto, px.ns)
}

func (px *PeerExchange) pushpull(ctx context.Context, s network.Stream) error {
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
		defer s.CloseWrite()

		gs, err := px.gs.Load()
		if err != nil {
			return err
		}

		enc := capnp.NewPackedEncoder(s)
		for _, g := range append(gs, px.gs.GetLocalRecord()) {
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
			remote view
			r      = io.LimitReader(s, int64(px.gs.MaxSize*mtu))
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

		return px.gs.MergeAndStore(remote)
	})

	return j.Wait()
}

func (px *PeerExchange) matcher() (func(s string) bool, error) {
	versionOK, err := helpers.MultistreamSemverMatcher(Proto)
	return func(s string) bool {
		return versionOK(path.Dir(s)) && px.ns == path.Base(s)
	}, err
}

func (px *PeerExchange) handler(ctx context.Context) func(s network.Stream) {
	const timeout = time.Second * 15

	return func(s network.Stream) {
		defer s.Close()

		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		log := px.log.
			WithField("stream", s.ID()).
			WithField("proto", s.Protocol()).
			WithField("conn", s.Conn().ID()).
			WithField("peer", s.Conn().RemotePeer()).
			WithField("timeout", timeout)

		log.Debug("handler started")
		defer func() { log.Debug("handler finished") }()

		if err := px.pushpull(ctx, s); err != nil {
			log = log.WithError(err)
		}
	}
}

/*
 * Set-up functions
 */

// hostComponents is  needed in order for fx to correctly handle
// the 'host.Host' interface.  Simply relying on 'fx.Supply' will
// use the type of the underlying host implementation, which may
// vary.
type hostComponents struct {
	fx.Out

	ID       peer.ID
	Bus      event.Bus
	Host     host.Host
	PrivKey  crypto.PrivKey
	CertBook ps.CertifiedAddrBook
}

func newHostComponents(h host.Host) func() (hostComponents, error) {
	return func() (cs hostComponents, err error) {
		cb, ok := ps.GetCertifiedAddrBook(h.Peerstore())
		if err = errNoSignedAddrs; ok {
			err = nil
			cs = hostComponents{
				Host:     h,
				ID:       h.ID(),
				Bus:      h.EventBus(),
				PrivKey:  h.Peerstore().PrivKey(h.ID()),
				CertBook: cb,
			}
		}

		return
	}
}

func newConfig(id peer.ID, opt []Option) (c Config) {
	c.Apply(opt)

	c.Log = c.Log.
		WithField("ns", c.NS).
		WithField("id", id)

	return
}

type pexParams struct {
	fx.In

	NS    string
	Log   log.Logger
	Host  host.Host
	Tick  time.Duration
	Store *gossipStore
}

func newPeerExchange(p pexParams, s fx.Shutdowner, lx fx.Lifecycle) *PeerExchange {
	px := &PeerExchange{
		h:       p.Host,
		ns:      p.NS,
		log:     p.Log,
		gs:      p.Store,
		runtime: s,
	}

	// Start gossip loop.
	var ticker *jitterbug.Ticker
	lx.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			ticker = jitterbug.New(p.Tick, jitterbug.Uniform{
				Min:    p.Tick / 2,
				Source: rand.New(rand.NewSource(time.Now().UnixNano())),
			})

			go func() {
				for range ticker.C {
					if err := px.gossip(ctx); err != nil {
						p.Log.WithError(err).Debug("gossip round failed")
					}
				}
			}()

			return nil
		},
		OnStop: func(context.Context) error {
			ticker.Stop()
			return nil
		},
	})

	return px
}

func newEvents(bus event.Bus, lx fx.Lifecycle) (e event.Emitter, s event.Subscription, err error) {
	if s, err = bus.Subscribe([]interface{}{
		new(event.EvtLocalAddressesUpdated),
		new(EvtViewUpdated),
	}); err == nil {
		lx.Append(closer(s))
	}

	if e, err = bus.Emitter(new(EvtViewUpdated), eventbus.Stateful); err == nil {
		lx.Append(closer(e))
	}

	return
}

type runParam struct {
	fx.In

	Log   log.Logger
	Host  host.Host
	Sub   event.Subscription
	Store *gossipStore
	CAB   ps.CertifiedAddrBook
	PeX   *PeerExchange
}

func (p runParam) Go(f func(log.Logger, <-chan interface{}, *gossipStore, ps.CertifiedAddrBook)) fx.Hook {
	return fx.Hook{
		OnStart: func(context.Context) error {
			go f(p.Log, p.Sub.Out(), p.Store, p.CAB)
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
				return p.Store.SetLocalRecord(ev.SignedPeerRecord)
			}

		case <-ctx.Done():
			return ctx.Err()
		}

		return errors.New("host shutting down")
	}})

	// Once the local record is safely stored in the gossipStore,
	// we can begin processing events normally in the background.
	lx.Append(p.Go(func(log log.Logger, events <-chan interface{}, s *gossipStore, c ps.CertifiedAddrBook) {
		for v := range events {
			switch ev := v.(type) {
			case event.EvtLocalAddressesUpdated:
				if err := s.SetLocalRecord(ev.SignedPeerRecord); err != nil {
					log.WithError(err).
						Error("error updating local record")
				}

			case EvtViewUpdated:
				for _, g := range ev {
					ok, err := c.ConsumePeerRecord(g.Envelope, ps.AddressTTL)
					if err != nil {
						log.WithError(err).
							Error("error adding gossip record to peerstore")
					}

					if ok {
						log.With(g).WithField("ttl", ps.AddressTTL).
							Trace("added gossip record to peerstore")
					}
				}

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
