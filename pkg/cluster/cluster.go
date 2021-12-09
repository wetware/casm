// Package cluster exports an asynchronously updated model of the swarm.
package cluster

import (
	"context"
	"time"

	"github.com/jpillora/backoff"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/wetware/casm/pkg/cluster/pulse"
	"github.com/wetware/casm/pkg/cluster/routing"
	"github.com/wetware/casm/pkg/util/service"
)

type PubSub interface {
	Join(string, ...pubsub.TopicOpt) (*pubsub.Topic, error)
	RegisterTopicValidator(string, interface{}, ...pubsub.ValidatorOpt) error
	UnregisterTopicValidator(string) error
}

type RoutingTable interface {
	View
	Advance(time.Time)
	Upsert(routing.Record) (created bool)
}

type View interface {
	Iter() routing.Iterator
	Lookup(peer.ID) (routing.Record, bool)
}

type Node struct {
	name string
	rt   RoutingTable
	a    announcer
	s    service.Set
}

// New cluster model.  It is safe to cancel 'ctx' after 'New' returns.
func New(ctx context.Context, ps PubSub, opt ...Option) (*Node, error) {
	n := &Node{}
	for _, option := range withDefault(opt) {
		option(n)
	}

	n.s = service.Set{n.newTopic(ps), &n.a, &clock{timer: n.rt}}
	return n, n.s.Start()
}

func (m *Node) Close() error         { return m.s.Close() }
func (m *Node) Topic() *pubsub.Topic { return m.a.t }
func (m *Node) View() View           { return m.rt }

func (m *Node) Bootstrap(ctx context.Context) (err error) {
	return m.a.announce(ctx)
}

func (m *Node) newTopic(ps PubSub) service.Service {
	var (
		cancel pubsub.RelayCancelFunc
	)

	return service.Set{
		// Update routing table via topic validator
		service.Hook{
			OnStart: func() (err error) {
				return ps.RegisterTopicValidator(m.name,
					pulse.NewValidator(m.rt))
			},
			OnClose: func() error {
				return ps.UnregisterTopicValidator(m.name)
			},
		},

		// Join and relay the topic
		service.Hook{
			OnStart: func() (err error) {
				if m.a.t, err = ps.Join(m.name); err == nil {
					cancel, err = m.a.t.Relay()
				}
				return
			},
			OnClose: func() error {
				cancel()
				return m.a.t.Close()
			},
		},
	}
}

type clock struct {
	cq    chan struct{}
	timer interface{ Advance(time.Time) }
}

func (c *clock) Start() error {
	c.cq = make(chan struct{})

	go func() {
		ticker := time.NewTicker(time.Millisecond * 10)
		defer ticker.Stop()

		for {
			select {
			case now := <-ticker.C:
				c.timer.Advance(now)

			case <-c.cq:
				return
			}
		}
	}()

	return nil
}

func (c *clock) Close() error {
	close(c.cq)
	return nil
}

type loggableBackoff struct{ backoff.Backoff }

func (b loggableBackoff) Loggable() map[string]interface{} {
	return map[string]interface{}{
		"attempt": int(b.Attempt()),
		"dur":     b.ForAttempt(b.Attempt()),
		"max_dur": b.Max,
	}
}
