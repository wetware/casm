package boot

import (
	"context"
	"time"

	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/peer"
)

type Cache struct {
	Match func(string) bool
	Cache discovery.Discovery
	Else  discovery.Discovery
}

func (c Cache) Advertise(ctx context.Context, ns string, opt ...discovery.Option) (time.Duration, error) {
	if c.Match(ns) {
		return c.Cache.Advertise(ctx, ns, opt...)
	}

	return c.Else.Advertise(ctx, ns, opt...)
}

func (c Cache) FindPeers(ctx context.Context, ns string, opt ...discovery.Option) (<-chan peer.AddrInfo, error) {
	if c.Match(ns) {
		return c.Cache.FindPeers(ctx, ns, opt...)
	}

	return c.Else.FindPeers(ctx, ns, opt...)
}
