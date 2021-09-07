package pex

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/ioutil"
	"math/rand"
	"path"
	"time"

	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/helpers"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	ps "github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/libp2p/go-libp2p-core/record"
	"go.uber.org/fx"

	"github.com/lthibault/jitterbug/v2"
	"github.com/lthibault/log"
	ctxutil "github.com/lthibault/util/ctx"
	syncutil "github.com/lthibault/util/sync"

	protoutil "github.com/wetware/casm/pkg/util/proto"
)

const (
	Version               = "0.0.0"
	baseProto protocol.ID = "/casm/pex"
	Proto     protocol.ID = baseProto + "/" + Version
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
	ns   string
	log  logger
	tick time.Duration

	h  host.Host
	pk crypto.PrivKey

	maxSize     int
	newSelector ViewSelectorFactory
	atomic      *atomicValues

	runtime fx.Shutdowner
}

// New peer exchange.
func New(h host.Host, opt ...Option) (pex PeerExchange, err error) {
	var ctx = ctxutil.FromChan(h.Network().Process().Closing())
	if err = ErrNoListenAddrs; len(h.Addrs()) > 0 {
		err = fx.New(fx.NopLogger,
			fx.Populate(&pex),
			fx.Supply(opt),
			fx.Provide(
				newAtomics,
				newPeerExchange,
				newSubscriptions,
				newHostComponents(h)),
			fx.Invoke(
				initGossipHandler,
				startEventLoop,
				startGossipLoop,
				waitReady)).
			Start(ctx)
	}

	return
}

func (px PeerExchange) String() string { return px.ns }
func (px PeerExchange) Close() error   { return px.runtime.Shutdown() }

func (px PeerExchange) Loggable() map[string]interface{} {
	return map[string]interface{}{
		"id":       px.h.ID(),
		"ns":       px.ns,
		"max_view": px.maxSize,
	}
}

// View is the set of peers contained in the passive view.
func (px PeerExchange) View() View {
	immut := px.atomic.view.Load() // DO NOT mutate
	view := make(View, len(immut))
	copy(view, immut)
	return view
}

// Join the namespace using a bootstrap peer.
//
// Join blocks until the underlying host is listening on at least one network
// address.
func (px PeerExchange) Join(ctx context.Context, boot peer.AddrInfo) error {
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

func (px PeerExchange) gossip(ctx context.Context) (err error) {
	view := px.atomic.view.Load() // do not mutate!
	peers := view.IDs()
	rand.Shuffle(len(peers), peers.Swap)

	for _, id := range peers {
		switch err = px.gossipOne(ctx, id); err.(type) {
		case nil:
			return
		case streamError:
			px.log.With(err.(log.Loggable)).Debug("unable to connect")
		default:
			return
		}
	}

	// we get here either if len(peers) == 0, or if all peers are unreachable.
	return errors.New("orphaned host")
}

func (px PeerExchange) gossipOne(ctx context.Context, id peer.ID) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second*15)
	defer cancel()

	s, err := px.h.NewStream(ctx, id, px.proto())
	if err != nil {
		return streamError{Peer: id, error: err}
	}

	return px.pushpull(ctx, s)
}

func (px PeerExchange) proto() protocol.ID {
	return protoutil.AppendStrings(Proto, px.ns)
}

func (px PeerExchange) pushpull(ctx context.Context, s network.Stream) error {
	// NOTE:  v MUST NOT be mutated!
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

		v := px.atomic.view.Load()

		// copy view and append local peer's gossip record
		rec := make(View, len(v), len(v)+1)
		copy(rec, v)
		rec = append(rec, px.atomic.record.Load())

		// marshal & sign 'rec'
		env, err := record.Seal(&rec, px.pk)
		if err != nil {
			return err
		}

		b, err := env.Marshal()
		if err != nil {
			return err
		}

		_, err = io.Copy(s, bytes.NewReader(b))
		return err
	})

	// pull
	j.Go(func() error {
		defer s.CloseRead()

		var remote View

		// defensively limit buffer size; assume 1kb per record
		b, err := ioutil.ReadAll(io.LimitReader(s, int64(px.maxSize)*1024))
		if err != nil {
			return err
		}

		env, err := record.ConsumeTypedEnvelope(b, &remote)
		if err != nil {
			return err
		}

		if err = remote.Validate(env); err != nil {
			//  TODO(security):  implement peer scoring system and punish peers
			//					 whose messages fail validation.
			return err
		}

		remote.incrHops()
		return px.mergeAndSelect(remote)
	})

	return j.Wait()
}

func (px PeerExchange) mergeAndSelect(remote View) error {
	px.atomic.Lock()
	defer px.atomic.Unlock()
	/*
	 *  CAUTION:  'local' MUST NOT be mutated!
	 */

	local := px.atomic.view.Load()
	sender := remote.last()
	selectv := px.newSelector(px.h, &sender, px.maxSize)

	return px.atomic.view.Store(selectv(px.merge(local, remote)))
}

func (px PeerExchange) merge(local, remote View) View {
	/*
	 * NOTE:
	 *   (a) we are holding the lock in 'px.atomic'
	 *   (b) 'local' MUST NOT mutate
	 */

	remote = append(remote, local...) // merge local view into remote.

	// Remove duplicate records.
	merged := remote[:0]
	id := px.h.ID()
	for _, g := range remote {
		// skip record if it came from us
		if g.PeerID == id {
			continue
		}

		have, found := merged.find(g)

		/* Select if:

		unique  ...   more recent   ...  less diffused  */
		if !found || g.Seq > have.Seq || g.Hop < have.Hop {
			merged = append(merged, g)
		}
	}

	return merged
}

func (px PeerExchange) updateLocalRecord(ev event.EvtLocalAddressesUpdated) error {
	g, err := NewGossipRecordFromEvent(ev)
	if err == nil {
		px.atomic.record.Store(g)
	}
	return err
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

	Bus      event.Bus
	Host     host.Host
	PrivKey  crypto.PrivKey
	CertBook ps.CertifiedAddrBook
}

func newHostComponents(h host.Host) func() hostComponents {
	return func() hostComponents {
		return hostComponents{
			Host:     h,
			Bus:      h.EventBus(),
			PrivKey:  h.Peerstore().PrivKey(h.ID()),
			CertBook: h.Peerstore().(ps.CertifiedAddrBook),
		}
	}
}

func newPeerExchange(h host.Host, k crypto.PrivKey, a *atomicValues, s fx.Shutdowner, opt []Option) PeerExchange {
	var pex = PeerExchange{
		h:       h,
		pk:      k,
		atomic:  a,
		runtime: s,
	}

	for _, option := range withDefaults(opt) {
		option(&pex)
	}

	return pex
}

func newAtomics(bus event.Bus, lx fx.Lifecycle) (vs *atomicValues, err error) {
	if vs, err = newAtomicValues(bus); err == nil {
		hook(lx, closer(vs))
	}
	return
}

func newSubscriptions(bus event.Bus, lx fx.Lifecycle) (sub event.Subscription, err error) {
	if sub, err = bus.Subscribe([]interface{}{
		new(event.EvtLocalAddressesUpdated),
		new(EvtViewUpdated),
	}); err == nil {
		hook(lx, closer(sub))
	}
	return
}

func initGossipHandler(px PeerExchange, lx fx.Lifecycle) error {
	const d = time.Second * 15
	var (
		ctx            = ctxutil.FromChan(px.h.Network().Process().Closing())
		versionOK, err = helpers.MultistreamSemverMatcher(Proto)
	)

	if err == nil {
		px.h.SetStreamHandlerMatch(Proto, func(s string) bool {
			return versionOK(path.Dir(s)) && px.ns == path.Base(s)
		}, func(s network.Stream) {
			defer s.Close()

			ctx, cancel := context.WithTimeout(ctx, d)
			defer cancel()

			if err := px.pushpull(ctx, s); err != nil {
				px.log.WithStream(s).WithError(err).
					Debug("error handling gosisp")
			}
		})
		hook(lx, deferred(func() { px.h.RemoveStreamHandler(Proto) }))
	}

	return err
}

func startGossipLoop(px PeerExchange, lx fx.Lifecycle) {
	ticker := jitterbug.New(px.tick, jitterbug.Uniform{
		Min:    px.tick / 2,
		Source: rand.New(rand.NewSource(time.Now().UnixNano())),
	})

	hook(lx,
		deferred(ticker.Stop),
		goWithContext(func(ctx context.Context) {
			for range ticker.C {
				if err := px.gossip(ctx); err != nil {
					px.log.WithError(err).Debug("gossip round failed")
				}
			}
		}))
}

func startEventLoop(px PeerExchange, sub event.Subscription, cb ps.CertifiedAddrBook, lx fx.Lifecycle) {
	hook(lx, goroutine(func() {
		for v := range sub.Out() {
			switch ev := v.(type) {
			case event.EvtLocalAddressesUpdated:
				if err := px.updateLocalRecord(ev); err != nil {
					px.log.WithError(err).Error("invalid peer record in event")
				}

			case EvtViewUpdated:
				for _, g := range ev {
					if _, err := cb.ConsumePeerRecord(g.Envelope, ps.AddressTTL); err != nil {
						px.log.WithError(err).Error("error storing gossiped PeerRecord")
					}
				}
			}
		}
	}))
}

func waitReady(px PeerExchange, bus event.Bus, lx fx.Lifecycle) {
	hook(lx, setup(func(ctx context.Context) error {
		sub, err := bus.Subscribe(new(EvtLocalRecordUpdated))
		if err == nil {
			defer sub.Close()

			select {
			case <-ctx.Done():
				err = ctx.Err()
			case _, ok := <-sub.Out():
				if !ok {
					err = errors.New("closing")
				}
			}
		}
		return err
	}))

}

type hookFunc func(*fx.Hook)

func hook(lx fx.Lifecycle, hfs ...hookFunc) {
	var h fx.Hook
	for _, apply := range hfs {
		apply(&h)
	}
	lx.Append(h)
}

func setup(f func(context.Context) error) hookFunc {
	return func(h *fx.Hook) { h.OnStart = f }
}

func goroutine(f func()) hookFunc {
	return goWithContext(func(context.Context) { go f() })
}

func goWithContext(f func(context.Context)) hookFunc {
	return setup(func(c context.Context) error {
		go f(c)
		return nil
	})
}

func deferred(f func()) hookFunc {
	return func(h *fx.Hook) {
		h.OnStop = func(context.Context) error {
			f()
			return nil
		}
	}
}

func closer(c io.Closer) hookFunc {
	return func(h *fx.Hook) {
		h.OnStop = func(context.Context) error {
			return c.Close()
		}
	}
}
