package cluster

import (
	"context"
	"io"
	"math/rand"
	"time"

	"capnproto.org/go/capnp/v3"
	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/lthibault/jitterbug/v2"
	"github.com/lthibault/treap"
	ctxutil "github.com/lthibault/util/ctx"
	"go.uber.org/fx"
)

type Iterator struct{ it *treap.Iterator }

func (i *Iterator) Next() (more bool)   { return i.it.Next() }
func (i *Iterator) More() bool          { return i.it.More() }
func (i *Iterator) Peer() peer.ID       { return idCastUnsafe(i.it.Key) }
func (i *Iterator) Deadline() time.Time { return timeCastUnsafe(i.it.Weight) }
func (i *Iterator) Record() Record      { return recordCastUnsafe(i.it.Value).Heartbeat.Record() }

type Model struct {
	t       *pubsub.Topic
	r       *routingTable
	runtime fx.Shutdowner
}

// New cluster.
func New(h host.Host, p *pubsub.PubSub, opt ...Option) (Model, error) {
	var m Model

	err := fx.New(fx.NopLogger,
		fx.Supply(p, opt),
		fx.Populate(&m),
		fx.Provide(
			newModel,
			newConfig,
			newHeartbeat,
			newEventEmitter,
			newRoutingTable,
			newClusterTopic,
			newHostComponents(h)),
		fx.Invoke(
			startClock,
			monitorNeighbors,
			start)).
		Start(hostctx(h))

	return m, err
}

func (m Model) Topic() *pubsub.Topic { return m.t }
func (m Model) Close() error         { return m.runtime.Shutdown() }

func (m Model) Contains(id peer.ID) bool { return m.r.Contains(id) }
func (m Model) Iter() Iterator           { return Iterator{handle.Iter(m.r.Load().n)} }

/*
 * Set-up functions
 */

type hostComponents struct {
	fx.Out

	Bus  event.Bus
	Host host.Host
}

func newHostComponents(h host.Host) func() hostComponents {
	return func() hostComponents {
		return hostComponents{
			Host: h,
			Bus:  h.EventBus(),
		}
	}
}

func newConfig(opt []Option) (c Config) {
	for _, option := range withDefault(opt) {
		option(&c)
	}
	return
}

func newModel(t *pubsub.Topic, r *routingTable, s fx.Shutdowner) Model {
	return Model{
		t:       t,
		r:       r,
		runtime: s,
	}
}

func newHeartbeat() (a announcement, hb heartbeat, err error) {
	if a, err = newAnnouncement(capnp.SingleSegment(nil)); err == nil {
		hb, err = a.NewHeartbeat()
	}
	return
}

func newEventEmitter(bus event.Bus, lx fx.Lifecycle) (e event.Emitter, err error) {
	if e, err = bus.Emitter(new(EvtMembershipChanged)); err == nil {
		hook(lx, closer(e))
	}
	return
}

func newRoutingTable(c Config, p *pubsub.PubSub, e event.Emitter, lx fx.Lifecycle) *routingTable {
	t := new(routingTable)
	t.Store(state{t: time.Now()})

	hook(lx,
		setup(func(context.Context) error {
			return p.RegisterTopicValidator(c.ns, t.NewValidator(e))
		}),
		teardown(func(context.Context) error {
			return p.UnregisterTopicValidator(c.ns)
		}))

	return t
}

func newClusterTopic(c Config, p *pubsub.PubSub, lx fx.Lifecycle) (t *pubsub.Topic, err error) {
	if t, err = p.Join(c.ns); err == nil {
		hook(lx, closer(t))
	}

	var cancel pubsub.RelayCancelFunc
	if cancel, err = t.Relay(); err == nil {
		hook(lx, deferred(cancel))
	}

	return
}

func monitorNeighbors(local host.Host, t *pubsub.Topic, r *routingTable, lx fx.Lifecycle) error {
	a, err := newAnnouncement(capnp.SingleSegment(nil))
	if err != nil {
		return err
	}

	h, err := t.EventHandler()
	if err != nil {
		return err
	}

	var (
		ev     = EvtMembershipChanged{Observer: local.ID()}
		cancel context.CancelFunc
	)

	hook(lx,
		deferred(func() { cancel() }),
		goroutineWithContext(func(ctx context.Context) {
			ctx, cancel = context.WithCancel(ctx)
			defer h.Cancel()

			for {
				if ev.PeerEvent, err = h.NextPeerEvent(ctx); err != nil {
					break // always a context error
				}

				// Don't spam the cluster if we already know about
				// the peer.  Others likely know about it already.
				if r.Contains(ev.Peer) {
					continue
				}

				switch ev.Type {
				case pubsub.PeerJoin:
					if err := a.SetJoin(ev.Peer); err != nil {
						panic(err)
					}

				case pubsub.PeerLeave:
					if err := a.SetLeave(ev.Peer); err != nil {
						panic(err)
					}
				}

				b, err := a.MarshalBinary()
				if err != nil {
					panic(err)
				}

				if err = t.Publish(ctx, b); err != nil {
					break
				}
			}

		}))

	return nil
}

func startClock(r *routingTable, lx fx.Lifecycle) {
	ticker := time.NewTicker(time.Millisecond * 10)
	hook(lx,
		deferred(ticker.Stop),
		goroutine(func() {
			for t := range ticker.C {
				r.Advance(t)
			}
		}))
}

func start(h host.Host, c Config, t *pubsub.Topic, a announcement, hb heartbeat, lx fx.Lifecycle) error {
	var ticker *jitterbug.Ticker

	// start the heartbeat loop
	hook(lx,
		deferred(func() { ticker.Stop() }),
		goroutineWithContext(func(ctx context.Context) {
			ticker = jitterbug.New(c.ttl/2, jitterbug.Uniform{
				Source: rand.New(rand.NewSource(time.Now().UnixNano())),
				Min:    c.ttl / 3,
			})

			for range ticker.C {
				c.hook(hb)
				if err := announce(ctx, t, a); err != nil {
					panic(err) // TODO:  log error
				}
			}
		}))

	// emit a one-off heartbeat to join the cluster
	hb.SetTTL(c.ttl)
	c.hook(hb)

	return announce(hostctx(h), t, a)
}

func announce(ctx context.Context, t *pubsub.Topic, a announcement) error {
	b, err := a.MarshalBinary()
	if err == nil {
		err = t.Publish(ctx, b)
	}

	return err
}

type hookFactory func(*fx.Hook)

func hook(lx fx.Lifecycle, hfs ...hookFactory) {
	var h fx.Hook
	for _, apply := range hfs {
		apply(&h)
	}
	lx.Append(h)
}

func setup(f func(context.Context) error) hookFactory {
	return func(h *fx.Hook) { h.OnStart = f }
}

func teardown(f func(context.Context) error) hookFactory {
	return func(h *fx.Hook) { h.OnStop = f }
}

func goroutine(f func()) hookFactory {
	return goroutineWithContext(func(context.Context) { go f() })
}

func goroutineWithContext(f func(context.Context)) hookFactory {
	return setup(func(c context.Context) error {
		go f(c)
		return nil
	})
}

func deferred(f func()) hookFactory {
	return func(h *fx.Hook) {
		h.OnStop = func(context.Context) error {
			f()
			return nil
		}
	}
}

func closer(c io.Closer) hookFactory {
	return func(h *fx.Hook) {
		h.OnStop = func(context.Context) error {
			return c.Close()
		}
	}
}

func hostctx(h host.Host) context.Context {
	return ctxutil.FromChan(h.Network().Process().Closing())
}
