package discover

import (
	"encoding/json"
	"fmt"

	"github.com/libp2p/go-libp2p-core/discovery"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/urfave/cli/v2"

	cmdutil "github.com/wetware/casm/internal/util/cmd"
	"github.com/wetware/casm/pkg/boot"
)

var (
	flags = []cli.Flag{
		&cli.StringFlag{
			Name:    "ns",
			Usage:   "cluster namespace",
			Value:   "casm",
			EnvVars: []string{"CASM_NS"},
		},
		&cli.StringFlag{
			Name:    "addr",
			Aliases: []string{"a"},
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
)

// Command constructor
func Command() *cli.Command {
	return &cli.Command{
		Name:   "discover",
		Usage:  "discover peers on the network",
		Flags:  flags,
		Action: run(),
	}
}

func run() cli.ActionFunc {
	return func(c *cli.Context) error {
		cancel := cmdutil.BindTimeout(c)
		defer cancel()

		d, err := newDiscoveryClient(c)
		if err != nil {
			return err
		}
		defer d.Close()

		enc := json.NewEncoder(c.App.Writer)
		if c.Bool("prettyprint") {
			enc.SetIndent("", "  ")
		}

		ps, err := d.FindPeers(c.Context, c.String("ns"), discovery.Limit(c.Int("n")))
		if err != nil {
			return err
		}

		for info := range ps {
			if err = enc.Encode(info); err != nil {
				return err
			}
		}

		return nil
	}
}

func newDiscoveryClient(c *cli.Context) (*boot.MulticastClient, error) {
	m, err := ma.NewMultiaddr(c.String("a"))
	if err != nil {
		return nil, fmt.Errorf("%w:  %s", err, m)
	}

	mc, err := boot.NewMulticastClient(m)
	return mc, err
}
