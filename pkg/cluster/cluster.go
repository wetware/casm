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
	"github.com/lthibault/log"
	"github.com/lthibault/treap"
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
func New(ctx context.Context, h host.Host, p *pubsub.PubSub, opt ...Option) (m Model, err error) {
	err = fx.New(fx.NopLogger,
		fx.Supply(p, opt),
		fx.Populate(&m),
		fx.Provide(
			newClock,
			newModel,
			newConfig,
			newEventEmitter,
			newRoutingTable,
			newClusterTopic,
			newTopicEventHandler,
			newHostComponents(h)),
		fx.Invoke(
			neighborhoodHook,
			heartbeatHooks)).
		Start(ctx)

	return
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

	ID   peer.ID
	Bus  event.Bus
	Host host.Host
}

func newHostComponents(h host.Host) func() hostComponents {
	return func() hostComponents {
		return hostComponents{
			ID:   h.ID(),
			Host: h,
			Bus:  h.EventBus(),
		}
	}
}

func newConfig(opt []Option) (c Config) {
	c.Apply(opt)
	return
}

func newModel(t *pubsub.Topic, r *routingTable, s fx.Shutdowner) Model {
	return Model{
		t:       t,
		r:       r,
		runtime: s,
	}
}

func newEventEmitter(bus event.Bus, lx fx.Lifecycle) (e event.Emitter, err error) {
	if e, err = bus.Emitter(new(EvtMembershipChanged)); err == nil {
		hook(lx, closer(e))
	}
	return
}

type clock struct {
	fx.Out
	Advance   *time.Ticker
	Heartbeat *jitterbug.Ticker
}

func newClock(ttl time.Duration, lx fx.Lifecycle) clock {
	ticker := time.NewTicker(time.Millisecond * 10)
	jitter := jitterbug.New(ttl/2, jitterbug.Uniform{
		Source: rand.New(rand.NewSource(time.Now().UnixNano())),
		Min:    ttl / 3,
	})

	hook(lx, deferred(func() {
		jitter.Stop()
		ticker.Stop()
	}))

	return clock{Advance: ticker, Heartbeat: jitter}
}

func newRoutingTable(ns string, p *pubsub.PubSub, e event.Emitter, t *time.Ticker, lx fx.Lifecycle) *routingTable {
	r := new(routingTable)
	r.Store(state{t: time.Now()})

	hook(lx,
		goroutine(func() {
			for t := range t.C {
				r.Advance(t)
			}
		}))

	hook(lx,
		setup(func(context.Context) error {
			return p.RegisterTopicValidator(ns, r.NewValidator(e))
		}),
		teardown(func(context.Context) error {
			return p.UnregisterTopicValidator(ns)
		}))

	return r
}

func newClusterTopic(ns string, p *pubsub.PubSub, lx fx.Lifecycle) (t *pubsub.Topic, err error) {
	if t, err = p.Join(ns); err == nil {
		hook(lx, closer(t))
	}

	var cancel pubsub.RelayCancelFunc
	if cancel, err = t.Relay(); err == nil {
		hook(lx, deferred(cancel))
	}

	return
}

func newTopicEventHandler(t *pubsub.Topic, lx fx.Lifecycle) (h *pubsub.TopicEventHandler, err error) {
	if h, err = t.EventHandler(); err == nil {
		hook(lx, deferred(h.Cancel))
	}

	return
}

// monitor the local neighborhood for join/leave events.
type monitor struct {
	fx.In

	Log log.Logger
	ID  peer.ID
	T   *pubsub.Topic
	H   *pubsub.TopicEventHandler
	R   *routingTable
}

func (m monitor) StartMonitorLoop(a announcement) func(context.Context) {
	return func(ctx context.Context) {
		var (
			err error
			ev  = EvtMembershipChanged{Observer: m.ID}
		)

		for {
			if ev.PeerEvent, err = m.H.NextPeerEvent(ctx); err != nil {
				break // always a context error
			}

			// Don't spam the cluster if we already know about
			// the peer.  Others likely know about it already.
			if m.R.Contains(ev.Peer) {
				continue
			}

			switch ev.Type {
			case pubsub.PeerJoin:
				if err := a.SetJoin(ev.Peer); err != nil {
					m.Log.WithError(err).Fatal("error writing to segment")
				}

			case pubsub.PeerLeave:
				if err := a.SetLeave(ev.Peer); err != nil {
					m.Log.WithError(err).Fatal("error writing to segment")
				}
			}

			b, err := a.MarshalBinary()
			if err != nil {
				m.Log.WithError(err).Fatal("error marshalling capnp message")
			}

			if err = m.T.Publish(ctx, b); err != nil {
				break
			}
		}
	}
}

// neighborhookHook attaches a lifecycle hook that notifies us of changes to the local
// peer's neighborhood.
func neighborhoodHook(m monitor, lx fx.Lifecycle) error {
	a, err := newAnnouncement(capnp.SingleSegment(nil))
	if err == nil {
		hook(lx, goroutineWithContext(m.StartMonitorLoop(a)))
	}

	return err
}

type announcer struct {
	fx.In

	Log    log.Logger
	T      *pubsub.Topic
	TTL    time.Duration
	Ticker *jitterbug.Ticker

	Hook Hook
}

func (ar announcer) AnnounceHeartbeat(ctx context.Context, a announcement, hb heartbeat) error {
	hb.SetTTL(ar.TTL)
	ar.Hook(hb)

	b, err := a.MarshalBinary()
	if err == nil {
		err = ar.T.Publish(ctx, b)
	}

	return err
}

func (ar announcer) StartHeartbeatLoop(a announcement, hb heartbeat) func(context.Context) {
	return func(ctx context.Context) {
		for range ar.Ticker.C {
			if err := ar.AnnounceHeartbeat(ctx, a, hb); err != nil {
				if err != context.Canceled {
					ar.Log.WithError(err).Error("failed to emit heartbeat")
				}
			}
		}
	}
}

func heartbeatHooks(ar announcer, lx fx.Lifecycle) error {
	a, err := newAnnouncement(capnp.SingleSegment(nil))
	if err != nil {
		return err
	}

	hb, err := a.NewHeartbeat()
	if err == nil {
		hook(lx,
			// Make a _blocking_ call to AnnounceHeartbeat before starting
			// the heartbeat loop. This ensures we are immediately visible
			// to our peers.
			setup(func(ctx context.Context) error {
				ctx, cancel := context.WithTimeout(ctx, time.Second*15)
				defer cancel()

				return ar.AnnounceHeartbeat(ctx, a, hb)
			}))

		hook(lx,
			goroutineWithContext(ar.StartHeartbeatLoop(a, hb)))
	}

	return err
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

func teardown(f func(context.Context) error) hookFunc {
	return func(h *fx.Hook) { h.OnStop = f }
}

func goroutine(f func()) hookFunc {
	return goroutineWithContext(func(context.Context) { go f() })
}

func goroutineWithContext(f func(context.Context)) hookFunc {
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
