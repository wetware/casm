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

type PubSub interface {
	ListPeers(topic string) []peer.ID
	Join(topic string, opts ...pubsub.TopicOpt) (*pubsub.Topic, error)
	RegisterTopicValidator(topic string, val interface{}, opts ...pubsub.ValidatorOpt) error
	UnregisterTopicValidator(topic string) error
}

type Manager struct{ ps PubSub }

func New(ps PubSub) Manager {
	return Manager{ps: ps}
}

// Join the cluster identified by the namespace ns.  The context is used only
// to connect to the cluster; cancelling ctx after Join has returned will NOT
// result in the local host leaving ns.  To leave a cluster, call its Close()
// method.
func (m Manager) Join(ctx context.Context, ns string, opt ...Option) (c Cluster, err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	app := fx.New(fx.NopLogger,
		fx.Populate(&c),
		fx.Supply(ns, m, opt),
		fx.Provide(
			newConfig,
			newCluster))

	if err = app.Start(ctx); err == nil {
		c.runtime = app
	}

	return
}

type Cluster struct {
	t       *pubsub.Topic
	runtime interface {
		Start(context.Context) error
		Stop(context.Context) error
	}
}

type clusterParam struct {
	fx.In

	Logger    log.Logger
	TTL       time.Duration
	Manager   Manager
	Topic     *pubsub.Topic
	OnPublish pulse.Hook
	Emitter   event.Emitter
}

func (p clusterParam) Log() log.Logger {
	return p.Logger.WithField("ns", p.Topic.String())
}

func (p clusterParam) Namespace() string {
	return p.Topic.String()
}

func (p clusterParam) HeartbeatTicker() (<-chan time.Time, func()) {
	jitter := jitterbug.New(p.TTL/2, jitterbug.Uniform{Min: p.TTL / 3})
	return jitter.C, jitter.Stop
}

func (p clusterParam) Validator(rt pulse.RoutingTable) fx.Hook {
	return fx.Hook{
		OnStart: func(context.Context) error {
			return p.Manager.ps.RegisterTopicValidator(p.Namespace(),
				pulse.NewValidator(rt, p.Emitter))
		},
		OnStop: func(context.Context) error {
			return p.Manager.ps.UnregisterTopicValidator(p.Namespace())
		},
	}
}

func (p clusterParam) Relay() fx.Hook {
	var cancel pubsub.RelayCancelFunc
	return fx.Hook{
		OnStart: func(context.Context) (err error) {
			cancel, err = p.Topic.Relay()
			return
		},
		OnStop: func(context.Context) error {
			cancel()
			return nil
		},
	}
}

type routingTable interface {
	Lookup(peer.ID) (routing.Record, bool)
}

func contains(rt routingTable, id peer.ID) bool {
	_, ok := rt.Lookup(id)
	return ok
}

var withReady = pubsub.WithReadiness(pubsub.MinTopicSize(1))

func (p clusterParam) Monitor(rt routingTable) fx.Hook {
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

			if h, err = p.Topic.EventHandler(); err != nil {
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
					if contains(rt, pe.Peer) {
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

					if err = p.Topic.Publish(ctx, b, withReady); err != nil {
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

func (p clusterParam) Tick(rt interface{ Advance(time.Time) }) fx.Hook {
	ticker := time.NewTicker(time.Millisecond * 100)

	return fx.Hook{
		OnStart: func(context.Context) error {
			go func() {
				for t := range ticker.C {
					rt.Advance(t)
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

func (p clusterParam) HeartbeatLoop() fx.Hook {
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
			if err = p.Topic.Publish(ctx, b, withReady); err == nil {
				ctx, cancel = context.WithCancel(context.Background())

				var tick <-chan time.Time
				tick, stop = p.HeartbeatTicker()

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

						if err = p.Topic.Publish(ctx, b, withReady); err == nil {
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

func newCluster(p clusterParam, lx fx.Lifecycle) Cluster {
	rt := routing.New()
	lx.Append(p.Validator(rt))
	lx.Append(p.Tick(rt))
	lx.Append(p.Relay())
	lx.Append(p.Monitor(rt))
	lx.Append(p.HeartbeatLoop())

	return Cluster{t: p.Topic}
}

// String returns the cluster's namespace.
func (c Cluster) String() string {
	return c.t.String()
}

func (c Cluster) Closer() error {
	return c.runtime.Stop(context.Background())
}

func newConfig(opt []Option) (c Config) {
	c.Apply(opt)
	return
}
