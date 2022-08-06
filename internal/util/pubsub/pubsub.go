package pubsubutil

// import (
// 	"github.com/libp2p/go-libp2p-core/discovery"
// 	"github.com/libp2p/go-libp2p-core/host"
// "github.com/libp2p/go-libp2p-core/discovery"

// 	"github.com/libp2p/go-libp2p-kad-dht/dual"
// 	pubsub "github.com/libp2p/go-libp2p-pubsub"
// 	"github.com/urfave/cli/v2"
// 	"github.com/wetware/casm/pkg/boot"
// 	"github.com/wetware/casm/pkg/pex"
// 	"go.uber.org/fx"
// )

// type Param struct {
// 	fx.In

// 	Host host.Host
// 	PeX  *pex.PeerExchange
// 	DHT  *dual.DHT
// }

// func (p Param) Discover() discovery.Discovery {
// 	return disc.NewRoutingDiscovery(p.DHT)
// }

// func New(c *cli.Context, p Param, lx fx.Lifecycle) (*pubsub.PubSub, error) {
// 	return pubsub.NewGossipSub(c.Context, p.Host,
// 		pubsub.WithPeerExchange(false), // we do our own via PeX.
// 		pubsub.WithDiscovery(boot.Dual{
// 			Boot:      bootstrapper(c, p.PeX),
// 			Discovery: p.Discover(),
// 		}))
// }

// func bootstrapper(c *cli.Context, d discovery.Discovery) bootService {
// 	return bootService{
// 		Context:   c,
// 		Discovery: d,
// 	}
// }

// type bootService struct {
// 	*cli.Context
// 	disc.Discovery
// }

// func (s bootService) String() string {
// 	return s.Context.String("ns")
// }
