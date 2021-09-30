package hostutil

import (
	"context"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-kad-dht/dual"
	routedhost "github.com/libp2p/go-libp2p/p2p/host/routed"
	"github.com/urfave/cli/v2"
	"go.uber.org/fx"
)

type Module struct {
	fx.Out

	DHT  *dual.DHT
	Host host.Host
}

func New(c *cli.Context, lx fx.Lifecycle) (m Module, err error) {
	m.Host, err = libp2p.New(c.Context)
	if err != nil {
		return
	}
	lx.Append(fx.Hook{
		OnStop: func(context.Context) error {
			return m.Host.Close()
		},
	})

	if m.DHT, err = dual.New(c.Context, m.Host); err != nil {
		return
	}

	m.Host = routedhost.Wrap(m.Host, m.DHT)

	return
}
