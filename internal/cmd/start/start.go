package start

import (
	"context"
	"errors"
	"io"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	discimpl "github.com/libp2p/go-libp2p-discovery"
	"github.com/libp2p/go-libp2p-kad-dht/dual"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	routedhost "github.com/libp2p/go-libp2p/p2p/host/routed"
	"github.com/lthibault/log"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/urfave/cli/v2"
	logutil "github.com/wetware/casm/internal/util/log"
	"github.com/wetware/casm/pkg/cluster"
	"github.com/wetware/casm/pkg/cluster/boot"
	"github.com/wetware/casm/pkg/pex"
	"go.uber.org/fx"
)

var (
	flags = []cli.Flag{
		&cli.StringFlag{
			Name:    "ns",
			Usage:   "cluster namespace",
			Value:   "casm",
			EnvVars: []string{"WW_NS"},
		},
		&cli.StringFlag{
			Name:    "discover",
			Aliases: []string{"d"},
			Usage:   "discovery service",
			Value:   "/multicast/ip4/228.8.8.8/udp/8822",
			EnvVars: []string{"WW_DISCOVER"},
		},
		&cli.StringSliceFlag{
			Name:    "join",
			Aliases: []string{"j"},
			Usage:   "join via static bootstrap address",
			EnvVars: []string{"WW_JOIN"},
		},
	}
)

// Command for 'client'.
func Command() *cli.Command {
	return &cli.Command{
		Name:   "start",
		Usage:  "start a host process",
		Flags:  flags,
		Action: run(),
	}
}

func run() cli.ActionFunc {
	return func(c *cli.Context) error {
		app := fx.New(fx.NopLogger,
			fx.Supply(c),
			fx.Provide(
				newLogger,
				newRoutedHost,
				newDiscovery,
				newCluster,
				newPubSub,
				newSub),
			fx.Invoke(func(log log.Logger, _ *cluster.Model, sub event.Subscription) {
				go func() {
					log.Info("started")
					defer log.Warn("shutting down")

					for v := range sub.Out() {
						switch ev := v.(type) {
						case cluster.EvtMembershipChanged:
							log.With(ev).Info("membership changed")

						case pex.EvtLocalRecordUpdated:
							log.With(ev).Debug("local record updated")

						case pex.EvtViewUpdated:
							log.With(ev).Trace("passive view updated")

						}
					}
				}()
			}))

		app.Run()

		return app.Err()
	}
}

func newRoutedHost(c *cli.Context, lx fx.Lifecycle) (host.Host, *dual.DHT, error) {
	h, err := libp2p.New(c.Context)
	if err != nil {
		return nil, nil, err
	}
	lx.Append(closer(h))

	dht, err := dual.New(c.Context, h)
	if err != nil {
		return nil, nil, err
	}
	lx.Append(closer(dht))

	return routedhost.Wrap(h, dht), dht, nil
}

func newDiscovery(c *cli.Context, log log.Logger, h host.Host, dht *dual.DHT, lx fx.Lifecycle) (discovery.Discovery, error) {
	d, err := bootstrapper(c, log, h, lx)
	if err != nil {
		return nil, err
	}

	px, err := pex.New(h, pex.WithNamespace(c.String("ns")))
	if err == nil {
		lx.Append(closer(px))
	}

	return boot.Dual{
		Boot:      boot.Cache{Discovery: d, Cache: px},
		Discovery: discimpl.NewRoutingDiscovery(dht),
	}, err
}

func bootstrapper(c *cli.Context, log log.Logger, h host.Host, lx fx.Lifecycle) (discovery.Discovery, error) {
	if addrs := c.StringSlice("join"); len(addrs) > 0 {
		return join(addrs)
	}

	if addr := c.String("discover"); addr != "" {
		d, err := discover(log, h, addr)
		if err == nil {
			lx.Append(closer(d))
		}
		return d, err
	}

	return nil, errors.New("must supply -join or -discover")
}

func newLogger(c *cli.Context, h host.Host) log.Logger {
	return logutil.Logger(c.Context).
		WithField("ns", h.ID())
}

func newPubSub(c *cli.Context, h host.Host, d discovery.Discovery) (*pubsub.PubSub, error) {
	return pubsub.NewGossipSub(c.Context, h,
		pubsub.WithPeerExchange(false),
		pubsub.WithDiscovery(d))
}

func newCluster(c *cli.Context, h host.Host, p *pubsub.PubSub) (cluster.Model, error) {
	return cluster.New(h, p, cluster.WithNamespace(c.String("ns")))
}

func newSub(h host.Host, lx fx.Lifecycle) (event.Subscription, error) {
	sub, err := h.EventBus().Subscribe([]interface{}{
		new(cluster.EvtMembershipChanged),
		new(pex.EvtLocalRecordUpdated),
		new(pex.EvtViewUpdated),
	})
	if err == nil {
		lx.Append(closer(sub))
	}

	return sub, err
}

func closer(c io.Closer) fx.Hook {
	return fx.Hook{
		OnStop: func(context.Context) error {
			return c.Close()
		},
	}
}

func join(ss []string) (_ boot.StaticAddrs, err error) {
	ms := make([]ma.Multiaddr, len(ss))
	for i, s := range ss {
		if ms[i], err = ma.NewMultiaddr(s); err != nil {
			return
		}
	}

	return peer.AddrInfosFromP2pAddrs(ms...)
}

func discover(log log.Logger, h host.Host, s string) (*boot.Multicast, error) {
	m, err := ma.NewMultiaddr(s)
	if err != nil {
		return nil, err
	}

	return boot.NewMulticast(h, m, boot.WithLogger(log))
}
