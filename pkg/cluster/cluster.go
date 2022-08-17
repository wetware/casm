// Package cluster exports an asynchronously updated model of the swarm.
package cluster

import (
	"context"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/wetware/casm/pkg/cluster/pulse"
	"github.com/wetware/casm/pkg/cluster/routing"
	"github.com/wetware/casm/pkg/cluster/view"
	"github.com/wetware/casm/pkg/util/service"
)

type PubSub interface {
	Join(string, ...pubsub.TopicOpt) (*pubsub.Topic, error)
	RegisterTopicValidator(string, interface{}, ...pubsub.ValidatorOpt) error
	UnregisterTopicValidator(string) error
}

type RoutingTable interface {
	Advance(time.Time)
	Upsert(routing.Record) (created bool)
	NewQuery() routing.Query
}

// type RecordBinder interface {
// 	Bind(api.View_Record) error // TODO(soon): api.View_Record is not exported
// }

type Node struct {
	ns string
	rt RoutingTable
	a  *announcer
	s  service.Set
}

// New cluster model.  It is safe to cancel 'ctx' after 'New' returns.
func New(ps PubSub, opt ...Option) (*Node, error) {
	n := &Node{a: newAnnouncer()}
	for _, option := range withDefault(opt) {
		option(n)
	}

	n.s = service.Set{n.newTopic(ps), n.a, &clock{timer: n.rt}}
	return n, n.s.Start()
}

func (n *Node) Close() error         { return n.s.Close() }
func (n *Node) String() string       { return n.ns }
func (n *Node) Topic() *pubsub.Topic { return n.a.t }
func (n *Node) View() view.View      { return view.View{Query: n.rt.NewQuery()} }

func (n *Node) Bootstrap(ctx context.Context, opt ...pubsub.PubOpt) error {
	return n.a.Emit(ctx, n.a.t, opt...)
}

func (n *Node) Loggable() map[string]interface{} {
	return map[string]interface{}{
		"ns": n.ns,
	}
}

func (n *Node) newTopic(ps PubSub) service.Service {
	var cancel pubsub.RelayCancelFunc

	return service.Set{
		// Update routing table via topic validator
		service.Hook{
			OnStart: func() (err error) {
				return ps.RegisterTopicValidator(n.ns,
					pulse.NewValidator(n.rt))
			},
			OnClose: func() error {
				return ps.UnregisterTopicValidator(n.ns)
			},
		},

		// Join and relay the topic
		service.Hook{
			OnStart: func() (err error) {
				if n.a.t, err = ps.Join(n.ns); err == nil {
					cancel, err = n.a.t.Relay()
				}
				return
			},
			OnClose: func() error {
				cancel()
				return n.a.t.Close()
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
