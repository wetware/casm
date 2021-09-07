package cluster

import (
	"context"
	"math/rand"
	"time"

	"capnproto.org/go/capnp/v3"
	"github.com/jbenet/goprocess"
	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/lthibault/jitterbug/v2"
	"github.com/lthibault/treap"
	ctxutil "github.com/lthibault/util/ctx"
	"go.uber.org/multierr"
)

type Iterator struct{ it *treap.Iterator }

func (i *Iterator) Next() (more bool)   { return i.it.Next() }
func (i *Iterator) More() bool          { return i.it.More() }
func (i *Iterator) Peer() peer.ID       { return idCastUnsafe(i.it.Key) }
func (i *Iterator) Deadline() time.Time { return timeCastUnsafe(i.it.Weight) }
func (i *Iterator) Record() Record      { return recordCastUnsafe(i.it.Value).Heartbeat.Record() }

type Model struct {
	ns  string
	ttl time.Duration

	h host.Host
	p *pubsub.PubSub
	t *pubsub.Topic

	m    routingTable
	proc goprocess.Process
	hook func(Heartbeat)
}

// New cluster.
func New(h host.Host, p *pubsub.PubSub, opt ...Option) (*Model, error) {
	var m = &Model{
		h:    h,
		p:    p,
		proc: goprocess.WithParent(h.Network().Process()),
	}

	// set options
	for _, option := range withDefault(opt) {
		option(m)
	}

	return m, start(m)
}

func (m *Model) String() string             { return m.t.String() }
func (m *Model) Process() goprocess.Process { return m.proc }
func (m *Model) Close() error               { return m.proc.Close() }

func (m *Model) Contains(id peer.ID) bool {
	_, ok := handle.Get(m.m.Load().n, id)
	return ok
}

func (m *Model) Iter() Iterator { return Iterator{handle.Iter(m.m.Load().n)} }

func (m *Model) announce(ctx context.Context, a announcement) error {
	b, err := a.MarshalBinary()
	if err == nil {
		err = m.t.Publish(ctx, b)
	}

	return err
}

/*
 * Set-up functions
 */

func start(m *Model) error {
	return (&initializer{}).Init(m)
}

type initFunc func(*Model)

type initializer struct {
	err error

	e      event.Emitter
	p      goprocess.Process
	cancel pubsub.RelayCancelFunc
	th     *pubsub.TopicEventHandler
	ch     chan EvtMembershipChanged

	joinLeave, heartbeat announcement
	hb                   heartbeat
}

func (init *initializer) Do(m *Model, fs ...initFunc) {
	for _, f := range fs {
		if init.err == nil {
			f(m)
		}
	}
}

// initialize the cluster
func (init *initializer) Init(m *Model) error {
	init.Do(m,
		init.ClusterTopic,
		init.Model,
		init.Events,
		init.Heartbeat)
	return init.err
}

func (init *initializer) ClusterTopic(m *Model) {
	init.Do(m,
		init.topic,
		init.emitter,
		init.validator,
		init.rootProc)
}

func (init *initializer) Model(m *Model) {
	init.Do(m,
		init.modelState,
		init.relay,
		init.clock)
}

func (init *initializer) Events(m *Model) {
	init.Do(m,
		init.topicEventHandler,
		init.monitorNeighbors,
		init.announcements,
		init.announceJoinLeave)
}

func (init *initializer) Heartbeat(m *Model) {
	init.Do(m,
		init.firstHeartbeat,
		init.heartbeatLoop)
}

func (init *initializer) topic(m *Model) {
	m.t, init.err = m.p.Join(m.ns)
}

func (init *initializer) emitter(m *Model) {
	init.e, init.err = m.h.EventBus().Emitter(new(EvtMembershipChanged))
}

func (init *initializer) validator(m *Model) {
	init.err = m.p.RegisterTopicValidator(m.ns, m.m.NewValidator(init.e))
}

func (init *initializer) rootProc(m *Model) {
	// Hang a subprocess onto c.proc because library users
	// may wish to set their own teardown functions on the
	// cluster process.
	init.p = goprocess.WithTeardown(func() error {
		return multierr.Combine(
			init.e.Close(),
			m.p.UnregisterTopicValidator(m.ns),
			m.t.Close())
	})

	m.proc.AddChild(init.p)
}

func (init *initializer) modelState(m *Model) {
	m.m.Store(state{t: time.Now()})
}

func (init *initializer) relay(m *Model) {
	init.cancel, init.err = m.t.Relay()
}

func (init *initializer) clock(m *Model) {
	init.p.Go(func(p goprocess.Process) {
		defer init.cancel()

		ticker := time.NewTicker(time.Millisecond * 10)
		defer ticker.Stop()

		for {
			select {
			case t := <-ticker.C:
				m.m.Advance(t)

			case <-p.Closing():
				return
			}
		}
	})
}

func (init *initializer) topicEventHandler(m *Model) {
	init.ch = make(chan EvtMembershipChanged, 32)
	init.th, init.err = m.t.EventHandler()
}

func (init *initializer) monitorNeighbors(m *Model) {
	init.p.Go(func(p goprocess.Process) {
		defer close(init.ch)
		defer init.th.Cancel()

		var (
			err error
			ctx = ctxutil.FromChan(p.Closing())
			ev  = EvtMembershipChanged{Observer: m.h.ID()}
		)

		for {
			if ev.PeerEvent, err = init.th.NextPeerEvent(ctx); err != nil {
				break // always a context error
			}

			// Don't spam the cluster if we already know about
			// the peer.  Others likely know about it already.
			if !m.Contains(ev.Peer) {
				select {
				case init.ch <- ev:
					_ = init.e.Emit(ev)
				case <-ctx.Done():
				}
			}
		}
	})
}

func (init *initializer) announcements(m *Model) {
	init.Do(m,
		func(m *Model) { init.joinLeave, init.err = newAnnouncement(capnp.SingleSegment(nil)) },
		func(m *Model) { init.heartbeat, init.err = newAnnouncement(capnp.SingleSegment(nil)) },
		func(m *Model) { init.hb, init.err = init.heartbeat.NewHeartbeat() })
}

func (init *initializer) announceJoinLeave(m *Model) {
	init.p.Go(func(p goprocess.Process) {
		ctx := ctxutil.FromChan(p.Closing())

		for ev := range init.ch {
			switch ev.Type {
			case pubsub.PeerJoin:
				if err := init.joinLeave.SetJoin(ev.Peer); err != nil {
					panic(err)
				}

			case pubsub.PeerLeave:
				if err := init.joinLeave.SetLeave(ev.Peer); err != nil {
					panic(err)
				}
			}

			switch err := m.announce(ctx, init.joinLeave); err {
			case nil, pubsub.ErrTopicClosed, context.Canceled:
			default:
				panic(err)
			}
		}
	})
}

func (init *initializer) firstHeartbeat(m *Model) {
	ctx, cancel := context.WithTimeout(ctxutil.FromChan(init.p.Closing()),
		time.Second*30)
	defer cancel()

	init.hb.SetTTL(m.ttl)
	m.hook(init.hb)

	init.err = m.announce(ctx, init.heartbeat)
}

func (init *initializer) heartbeatLoop(m *Model) {
	init.p.Go(func(p goprocess.Process) {
		ticker := jitterbug.New(m.ttl/2, jitterbug.Uniform{
			Source: rand.New(rand.NewSource(time.Now().UnixNano())),
			Min:    m.ttl / 3,
		})
		defer ticker.Stop()

		ctx := ctxutil.FromChan(p.Closing())

		for {
			select {
			case <-ticker.C:
				m.hook(init.hb)

				switch err := m.announce(ctx, init.heartbeat); err {
				case nil, pubsub.ErrTopicClosed, context.Canceled:
				default:
					panic(err)
				}

			case <-p.Closing():
				return
			}
		}
	})
}
