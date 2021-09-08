package discover

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/libp2p/go-libp2p-core/discovery"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/urfave/cli/v2"
	"go.uber.org/fx"

	logutil "github.com/wetware/casm/internal/util/log"
	"github.com/wetware/casm/pkg/boot"
)

var (
	app *fx.App
	enc *json.Encoder
	d   discovery.Discoverer
)

var flags = []cli.Flag{
	&cli.StringFlag{
		Name:    "ns",
		Usage:   "cluster namespace",
		Value:   "casm",
		EnvVars: []string{"CASM_NS"},
	},
	&cli.StringFlag{
		Name:    "discover",
		Aliases: []string{"d"},
		Usage:   "discovery service multiaddress",
		Value:   "/multicast/ip4/228.8.8.8/udp/8822",
	},
	&cli.DurationFlag{
		Name:    "timeout",
		Aliases: []string{"t"},
		Usage:   "stop after t seconds",
	},
	&cli.IntFlag{
		Name:    "number",
		Aliases: []string{"n"},
		Usage:   "number of records to return (0 = stream)",
		Value:   1,
	},
}

// Command constructor
func Command() *cli.Command {
	return &cli.Command{
		Name:   "discover",
		Usage:  "discover peers on the network",
		Flags:  flags,
		Before: before(),
		After:  after(),
		Action: run(),
	}
}

func before() cli.BeforeFunc {
	return func(c *cli.Context) error {
		app = fx.New(fx.NopLogger,
			fx.Supply(c),
			fx.Populate(&d, &enc),
			fx.Provide(
				newEncoder,
				newTransport,
				newDiscoveryClient))
		return app.Start(c.Context)
	}
}

func after() cli.AfterFunc {
	return func(c *cli.Context) error {
		return app.Stop(c.Context)
	}
}

func run() cli.ActionFunc {
	return func(c *cli.Context) error {
		ctx, cancel := maybeTimeout(c)
		defer cancel()

		if c.Duration("t") > 0 {
			ctx, cancel = context.WithTimeout(c.Context, c.Duration("t"))
			defer cancel()
		}

		ps, err := d.FindPeers(ctx, c.String("ns"), discovery.Limit(c.Int("n")))
		if err != nil {
			return err
		}

		for info := range ps {
			if err := enc.Encode(info); err != nil {
				return err
			}
		}

		return nil
	}
}

func maybeTimeout(c *cli.Context) (context.Context, context.CancelFunc) {
	if c.Duration("t") > 0 {
		return context.WithTimeout(c.Context, c.Duration("t"))
	}

	return c.Context, func() {}
}

func newEncoder(c *cli.Context) *json.Encoder {
	enc := json.NewEncoder(c.App.Writer)
	if c.Bool("prettyprint") {
		enc.SetIndent("", "  ")
	}

	return enc
}

func newDiscoveryClient(c *cli.Context, t boot.Transport, lx fx.Lifecycle) (discovery.Discoverer, error) {
	d, err := boot.NewMulticastClient(
		boot.WithTransport(t),
		boot.WithLogger(logutil.New(c)))

	if err == nil {
		lx.Append(closer(d))
	}

	return d, err
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
