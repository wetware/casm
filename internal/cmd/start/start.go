package start

import (
	"context"
	"errors"
	"fmt"
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
	"github.com/wetware/casm/pkg/boot"
	"github.com/wetware/casm/pkg/cluster"
	"github.com/wetware/casm/pkg/pex"
	"go.uber.org/fx"
)

var (
	app *fx.App

	logger log.Logger
	sub    event.Subscription
	m      cluster.Model
)

var flags = []cli.Flag{
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

// Command for 'client'.
func Command() *cli.Command {
	return &cli.Command{
		Name:   "start",
		Usage:  "start a host process",
		Flags:  flags,
		Before: before(),
		Action: run(),
	}
}

func before() cli.BeforeFunc {
	return func(c *cli.Context) error {
		app = fx.New(fx.NopLogger,
			fx.Supply(c),
			fx.Populate(&logger, &sub, &m),
			fx.Provide(
				newLogger,
				newRoutedHost,
				newTransport,
				newBootstrapper(c),
				newDiscovery,
				newCluster,
				newPubSub,
				newSub))
		return app.Start(c.Context)
	}
}

func run() cli.ActionFunc {
	return func(c *cli.Context) error {
		logger.Info("started")
		defer logger.Warn("shutting down")

		for {
			select {
			case v := <-sub.Out():
				switch ev := v.(type) {
				case cluster.EvtMembershipChanged:
					logger.With(ev).Info("membership changed")

				case pex.EvtViewUpdated:
					logger.With(ev).Trace("passive view updated")
				}

			case <-app.Done():
				return app.Stop(context.Background())
			}
		}
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

func newLogger(c *cli.Context, h host.Host) log.Logger {
	return logutil.New(c).
		WithField("ns", c.String("ns")).
		WithField("id", h.ID())
}

func newDiscovery(c *cli.Context, h host.Host, dht *dual.DHT, b discovery.Discovery, lx fx.Lifecycle) (boot.Dual, error) {
	px, err := pex.New(h, pex.WithNamespace(c.String("ns")))
	if err == nil {
		lx.Append(closer(px))
	}

	return boot.Dual{
		Boot:      boot.Cache{Discovery: b, Cache: px},
		Discovery: discimpl.NewRoutingDiscovery(dht),
	}, err
}

func newPubSub(c *cli.Context, h host.Host, d boot.Dual) (*pubsub.PubSub, error) {
	return pubsub.NewGossipSub(c.Context, h,
		pubsub.WithPeerExchange(false),
		pubsub.WithDiscovery(d))
}

func newCluster(c *cli.Context, h host.Host, p *pubsub.PubSub) (cluster.Model, error) {
	return cluster.New(c.Context, h, p,
		cluster.WithNamespace(c.String("ns")))
}

func newSub(h host.Host, lx fx.Lifecycle) (event.Subscription, error) {
	sub, err := h.EventBus().Subscribe([]interface{}{
		new(cluster.EvtMembershipChanged),
		new(pex.EvtViewUpdated),
	})
	if err == nil {
		lx.Append(closer(sub))
	}

	return sub, err
}

func newBootstrapper(c *cli.Context) interface{} {
	if len(c.String("d")) > 0 {
		return newBootService
	}

	if len(c.StringSlice("join")) > 0 {
		return newStaticBoot
	}

	return func() (discovery.Discovery, error) {
		return nil, errors.New("must supply -join or -discover")
	}
}

func newBootService(log log.Logger, h host.Host, t boot.Transport, lx fx.Lifecycle) (discovery.Discovery, error) {
	m, err := boot.NewMulticast(h,
		boot.WithLogger(log),
		boot.WithTransport(t))

	if err == nil {
		lx.Append(closer(m))
	}

	return m, err
}

func newStaticBoot(c *cli.Context) (discovery.Discovery, error) {
	var as boot.StaticAddrs

	for _, s := range c.StringSlice("join") {
		m, err := ma.NewMultiaddr(s)
		if err != nil {
			return nil, err
		}

		info, err := peer.AddrInfoFromP2pAddr(m)
		if err != nil {
			return nil, err
		}

		as = append(as, *info)
	}

	return as, nil
}

func newTransport(c *cli.Context) (boot.Transport, error) {
	m, err := ma.NewMultiaddr(c.String("d"))
	if err != nil {
		return nil, fmt.Errorf("%w:  %s", err, m)
	}

	return boot.NewTransport(m)
}

func closer(c io.Closer) fx.Hook {
	return fx.Hook{
		OnStop: func(context.Context) error { return c.Close() },
	}
}
