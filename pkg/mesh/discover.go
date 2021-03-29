package mesh

import (
	"context"

	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/peer"
)

// StaticAddrs for cluster discovery
type StaticAddrs []peer.AddrInfo

// Loggable representation
func (as StaticAddrs) Loggable() map[string]interface{} {
	return map[string]interface{}{
		"boot_strategy": "static_addrs",
		"boot_addrs":    as,
	}
}

// FindPeers converts the static addresses into AddrInfos.
func (as StaticAddrs) FindPeers(_ context.Context, _ string, opt ...discovery.Option) (<-chan peer.AddrInfo, error) {
	options := discovery.Options{Limit: len(as)}
	if err := options.Apply(opt...); err != nil {
		return nil, err
	}

	ch := make(chan peer.AddrInfo, min(options.Limit, len(as)))
	for _, p := range as {
		ch <- p
	}
	close(ch)

	return ch, nil
}

func min(i, j int) int {
	if i < j {
		return i
	}

	return j
}
