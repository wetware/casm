// Package cluster provides a clustering service based on pubsub.
package cluster

import (
	"context"
	"math/rand"
	"time"

	"capnproto.org/go/capnp/v3"
	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/lthibault/jitterbug/v2"
	"github.com/lthibault/log"
	"github.com/wetware/casm/pkg/cluster/pulse"
	"github.com/wetware/casm/pkg/cluster/routing"
	"go.uber.org/fx"
)

func init() { rand.Seed(time.Now().UnixNano()) }

type RoutingTable interface {
	Advance(time.Time)
	Iter() routing.Iterator
	Upsert(routing.Record) bool
	Lookup(peer.ID) (routing.Record, bool)
}

type PubSub interface {
	ListPeers(topic string) []peer.ID
	Join(topic string, opts ...pubsub.TopicOpt) (*pubsub.Topic, error)
	RegisterTopicValidator(topic string, val interface{}, opts ...pubsub.ValidatorOpt) error
	UnregisterTopicValidator(topic string) error
}

type Param struct {
	fx.In

	NS        string
	Logger    log.Logger
	TTL       time.Duration
	Routing   RoutingTable
	PubSub    PubSub
	OnPublish pulse.Hook
	Emitter   event.Emitter
}

func (p Param) Log() log.Logger {
	return p.Logger.WithField("ns", p.NS)
}

func (p Param) heartbeatTicker() (<-chan time.Time, func()) {
	jitter := jitterbug.New(p.TTL/2, jitterbug.Uniform{Min: p.TTL / 3})
	return jitter.C, jitter.Stop
}

func (p Param) validator() fx.Hook {
	return fx.Hook{
		OnStart: func(context.Context) error {
			return p.PubSub.RegisterTopicValidator(p.NS,
				pulse.NewValidator(p.Routing, p.Emitter))
		},
		OnStop: func(context.Context) error {
			return p.PubSub.UnregisterTopicValidator(p.NS)
		},
	}
}

func (p Param) relay(t *pubsub.Topic) fx.Hook {
	var cancel pubsub.RelayCancelFunc
	return fx.Hook{
		OnStart: func(context.Context) (err error) {
			cancel, err = t.Relay()
			return
		},
		OnStop: func(context.Context) error {
			cancel()
			return nil
		},
	}
}

func contains(rt routingTable, id peer.ID) bool {
	_, ok := rt.Lookup(id)
	return ok
}

var withReady = pubsub.WithReadiness(pubsub.MinTopicSize(1))

func (p Param) monitor(t *pubsub.Topic) fx.Hook {
	var (
		h           *pubsub.TopicEventHandler
		ctx, cancel = context.WithCancel(context.Background())
	)

	return fx.Hook{
		OnStart: func(context.Context) error {
			ev, err := pulse.NewClusterEvent(capnp.SingleSegment(nil))
			if err != nil {
				return err
			}

			if h, err = t.EventHandler(); err != nil {
				return err
			}

			go func() {
				for {
					pe, err := h.NextPeerEvent(ctx)
					if err != nil {
						break
					}

					// Don't spam the cluster if we already know about
					// the peer.  Others likely know about it already.
					if contains(p.Routing, pe.Peer) {
						continue
					}

					switch pe.Type {
					case pubsub.PeerJoin:
						if err := ev.SetJoin(pe.Peer); err != nil {
							p.Log().WithError(err).Fatal("error writing to segment")
						}

					case pubsub.PeerLeave:
						if err := ev.SetLeave(pe.Peer); err != nil {
							p.Log().WithError(err).Fatal("error writing to segment")
						}
					}

					b, err := ev.MarshalBinary()
					if err != nil {
						p.Log().WithError(err).Fatal("error marshalling capnp message")
					}

					if err = t.Publish(ctx, b, withReady); err != nil {
						break
					}

				}
			}()

			return err
		},
		OnStop: func(context.Context) error {
			cancel()
			h.Cancel()
			return nil
		},
	}
}

func (p Param) tick() fx.Hook {
	ticker := time.NewTicker(time.Millisecond * 100)

	return fx.Hook{
		OnStart: func(context.Context) error {
			go func() {
				for t := range ticker.C {
					p.Routing.Advance(t)
				}
			}()
			return nil
		},
		OnStop: func(context.Context) error {
			ticker.Stop()
			return nil
		},
	}
}

func (p Param) heartbeatLoop(t *pubsub.Topic) fx.Hook {
	var (
		stop   func() // stop the ticker
		cancel context.CancelFunc
	)

	return fx.Hook{
		OnStart: func(ctx context.Context) error {
			ev, err := pulse.NewClusterEvent(capnp.SingleSegment(nil))
			if err != nil {
				return err
			}

			h, err := ev.NewHeartbeat()
			if err != nil {
				return err
			}

			h.SetTTL(p.TTL)
			p.OnPublish(h)

			b, err := ev.MarshalBinary()
			if err != nil {
				return err
			}

			// publish once to join the cluster
			if err = t.Publish(ctx, b, withReady); err == nil {
				ctx, cancel = context.WithCancel(context.Background())

				var tick <-chan time.Time
				tick, stop = p.heartbeatTicker()

				go func() {
					p.Log().Debug("started heartbeat loop")
					defer p.Log().Debug("exited heartbeat loop")

					for range tick {
						h.SetTTL(p.TTL)
						p.OnPublish(h)

						if b, err = ev.MarshalBinary(); err != nil {
							p.Log().
								WithError(err).
								Error("error marshalling heartbeat event")
							continue
						}

						if err = t.Publish(ctx, b, withReady); err == nil {
							continue
						}

						if err != context.Canceled {
							p.Log().WithError(err).Error("heartbeat publication failed")
						}

						return
					}
				}()
			}

			return err
		},

		OnStop: func(context.Context) error {
			stop()
			cancel()
			return nil
		},
	}
}

type routingTable interface {
	Lookup(peer.ID) (routing.Record, bool)
	Iter() routing.Iterator
}

type Cluster struct {
	t *pubsub.Topic
	routingTable

	runtime interface {
		Start(context.Context) error
		Stop(context.Context) error
	}
}

func New(p Param, lx fx.Lifecycle) (Cluster, error) {
	t, err := p.PubSub.Join(p.NS)
	if err == nil {
		lx.Append(p.validator())
		lx.Append(p.tick())
		lx.Append(p.relay(t))
		lx.Append(p.monitor(t))
		lx.Append(p.heartbeatLoop(t))
	}

	return Cluster{
		routingTable: p.Routing,
		t:            t,
	}, err
}

// String returns the cluster's namespace.
func (c Cluster) String() string {
	return c.t.String()
}

func (c Cluster) Topic() *pubsub.Topic { return c.t }

func (c Cluster) Closer() error {
	return c.runtime.Stop(context.Background())
}
