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

type Iterator treap.Iterator

func (i *Iterator) Next() (more bool)   { return (*treap.Iterator)(i).Next() }
func (i *Iterator) More() bool          { return (*treap.Iterator)(i).More() }
func (i *Iterator) Peer() peer.ID       { return idCastUnsafe((*treap.Iterator)(i).Key) }
func (i *Iterator) Deadline() time.Time { return timeCastUnsafe((*treap.Iterator)(i).Weight) }

func (i *Iterator) Record() Record {
	return recordCastUnsafe((*treap.Iterator)(i).Value).Heartbeat.Record()
}

type Cluster struct {
	ns  string
	ttl time.Duration

	h host.Host
	p *pubsub.PubSub
	t *pubsub.Topic

	m    model
	proc goprocess.Process
	hook func(Heartbeat)
}

// New cluster.
func New(h host.Host, p *pubsub.PubSub, opt ...Option) (*Cluster, error) {
	var c = &Cluster{
		h:    h,
		p:    p,
		proc: goprocess.WithParent(h.Network().Process()),
	}

	// set options
	for _, option := range withDefault(opt) {
		option(c)
	}

	return c, start(c)
}

func (c *Cluster) String() string             { return c.t.String() }
func (c *Cluster) Process() goprocess.Process { return c.proc }
func (c *Cluster) Close() error               { return c.proc.Close() }

func (c *Cluster) Contains(id peer.ID) bool {
	_, ok := handle.Get(c.m.Load().n, id)
	return ok
}

func (c *Cluster) Iter() *Iterator { return (*Iterator)(handle.Iter(c.m.Load().n)) }

// block until at least one peer is connected
var pubOpt = pubsub.WithReadiness(pubsub.MinTopicSize(1))

func (c *Cluster) announce(ctx context.Context, a announcement) error {
	b, err := a.MarshalBinary()
	if err == nil {
		err = c.t.Publish(ctx, b, pubOpt)
	}

	return err
}

/*
 * Set-up functions
 */

func start(c *Cluster) error {
	return (&initializer{}).Init(c)
}

type initFunc func(*Cluster)

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

func (init *initializer) Do(c *Cluster, fs ...initFunc) {
	for _, f := range fs {
		if init.err == nil {
			f(c)
		}
	}
}

// initialize the cluster
func (init *initializer) Init(c *Cluster) error {
	init.Do(c,
		init.ClusterTopic,
		init.Model,
		init.Events,
		init.Heartbeat)
	return init.err
}

func (init *initializer) ClusterTopic(c *Cluster) {
	init.Do(c,
		init.topic,
		init.emitter,
		init.validator,
		init.rootProc)
}

func (init *initializer) Model(c *Cluster) {
	init.Do(c,
		init.modelState,
		init.relay,
		init.clock)
}

func (init *initializer) Events(c *Cluster) {
	init.Do(c,
		init.topicEventHandler,
		init.monitorNeighbors,
		init.announcements,
		init.announceJoinLeave)
}

func (init *initializer) Heartbeat(c *Cluster) {
	init.Do(c,
		init.firstHeartbeat,
		init.heartbeatLoop)
}

func (init *initializer) topic(c *Cluster) {
	c.t, init.err = c.p.Join(c.ns)
}

func (init *initializer) emitter(c *Cluster) {
	init.e, init.err = c.h.EventBus().Emitter(new(EvtMembershipChanged))
}

func (init *initializer) validator(c *Cluster) {
	init.err = c.p.RegisterTopicValidator(c.ns, c.m.NewValidator(init.e))
}

func (init *initializer) rootProc(c *Cluster) {
	// Hang a subprocess onto c.proc because library users
	// may wish to set their own teardown functions on the
	// cluster process.
	init.p = goprocess.WithTeardown(func() error {
		return multierr.Combine(
			init.e.Close(),
			c.p.UnregisterTopicValidator(c.ns),
			c.t.Close())
	})

	c.proc.AddChild(init.p)
}

func (init *initializer) modelState(c *Cluster) {
	c.m.Store(state{t: time.Now()})
}

func (init *initializer) relay(c *Cluster) {
	init.cancel, init.err = c.t.Relay()
}

func (init *initializer) clock(c *Cluster) {
	init.p.Go(func(p goprocess.Process) {
		defer init.cancel()

		ticker := time.NewTicker(time.Millisecond * 10)
		defer ticker.Stop()

		for {
			select {
			case t := <-ticker.C:
				c.m.Advance(t)

			case <-p.Closing():
				return
			}
		}
	})
}

func (init *initializer) topicEventHandler(c *Cluster) {
	init.ch = make(chan EvtMembershipChanged, 32)
	init.th, init.err = c.t.EventHandler()
}

func (init *initializer) monitorNeighbors(c *Cluster) {
	init.p.Go(func(p goprocess.Process) {
		defer close(init.ch)
		defer init.th.Cancel()

		var (
			err error
			ctx = ctxutil.FromChan(p.Closing())
			ev  = EvtMembershipChanged{Observer: c.h.ID()}
		)

		for {
			if ev.PeerEvent, err = init.th.NextPeerEvent(ctx); err != nil {
				break // always a context error
			}

			// Don't spam the cluster if we already know about
			// the peer.  Others likely know about it already.
			if !c.Contains(ev.Peer) {
				select {
				case init.ch <- ev:
					_ = init.e.Emit(ev)
				case <-ctx.Done():
				}
			}
		}
	})
}

func (init *initializer) announcements(c *Cluster) {
	init.Do(c,
		func(c *Cluster) { init.joinLeave, init.err = newAnnouncement(capnp.SingleSegment(nil)) },
		func(c *Cluster) { init.heartbeat, init.err = newAnnouncement(capnp.SingleSegment(nil)) },
		func(c *Cluster) { init.hb, init.err = init.heartbeat.NewHeartbeat() })
}

func (init *initializer) announceJoinLeave(c *Cluster) {
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

			switch err := c.announce(ctx, init.joinLeave); err {
			case nil, pubsub.ErrTopicClosed, context.Canceled:
			default:
				panic(err)
			}
		}
	})
}

func (init *initializer) firstHeartbeat(c *Cluster) {
	ctx, cancel := context.WithTimeout(ctxutil.FromChan(init.p.Closing()),
		time.Second*30)
	defer cancel()

	init.hb.SetTTL(c.ttl)
	c.hook(init.hb)

	init.err = c.announce(ctx, init.heartbeat)
}

func (init *initializer) heartbeatLoop(c *Cluster) {
	init.p.Go(func(p goprocess.Process) {
		ticker := jitterbug.New(c.ttl/2, jitterbug.Uniform{
			Source: rand.New(rand.NewSource(time.Now().UnixNano())),
			Min:    c.ttl / 3,
		})
		defer ticker.Stop()

		ctx := ctxutil.FromChan(p.Closing())

		for {
			select {
			case <-ticker.C:
				c.hook(init.hb)

				switch err := c.announce(ctx, init.heartbeat); err {
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
