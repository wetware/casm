package pexutil

import (
	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/lthibault/log"
	"github.com/urfave/cli/v2"
	"github.com/wetware/casm/pkg/pex"
	"go.uber.org/fx"
)

type Param struct {
	fx.In

	Log  log.Logger
	Host host.Host
	Boot discovery.Discovery `json:"boot"`
}

func New(c *cli.Context, p Param) (*pex.PeerExchange, error) {
	return pex.New(c.Context, p.Host,
		pex.WithLogger(p.Log),
		pex.WithDiscovery(p.Boot))
}
