package bootutil

import (
	"errors"

	"github.com/libp2p/go-libp2p/core/discovery"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/urfave/cli/v2"
	"github.com/wetware/casm/pkg/boot"
	"go.uber.org/fx"
)

type Module struct {
	fx.Out

	Boot discovery.Discovery
}

func New(c *cli.Context) (d discovery.Discovery, err error) {
	if c.IsSet("join") {
		d, err = join(c)
	} else {
		d, err = discover(c)
	}

	return
}

func join(c *cli.Context) (_ boot.StaticAddrs, err error) {
	var ms []ma.Multiaddr
	for i, s := range c.StringSlice("join") {
		if ms[i], err = ma.NewMultiaddr(s); err != nil {
			break
		}
	}

	return peer.AddrInfosFromP2pAddrs(ms...)
}

func discover(c *cli.Context) (discovery.Discovery, error) {
	return nil, errors.New("bootutil.discover() NOT IMPLEMENTED")
}
