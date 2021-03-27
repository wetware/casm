package mesh

import (
	"context"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"
)

var (
	_ Bootstrapper = (*StaticAddrs)(nil)
)

// StaticAddrs for cluster discovery
type StaticAddrs []multiaddr.Multiaddr

// Loggable representation
func (as StaticAddrs) Loggable() map[string]interface{} {
	return map[string]interface{}{
		"boot_strategy": "static_addrs",
		"boot_addrs":    as,
	}
}

// Boot converts the static addresses into AddrInfos
func (as StaticAddrs) Bootstrap(context.Context) (<-chan peer.AddrInfo, error) {
	ps, err := peer.AddrInfosFromP2pAddrs(as...)
	if err != nil {
		return nil, err
	}

	ch := make(chan peer.AddrInfo, len(ps))
	for _, p := range ps {
		ch <- p
	}
	close(ch)

	return ch, err
}
