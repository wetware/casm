// Package boot contains discovery services suitable for cluster bootstrap.
package boot

import (
	"context"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/peer"
	ps "github.com/libp2p/go-libp2p-core/peerstore"
	ma "github.com/multiformats/go-multiaddr"
)

var (
	_ discovery.Discoverer = StaticAddrs(nil)
)

// StaticAddrs is a set of bootstrap-peer addresses.
type StaticAddrs []peer.AddrInfo

// NewStaticAddrs from one or more multiaddrs.
func NewStaticAddrs(as ...ma.Multiaddr) (StaticAddrs, error) {
	return peer.AddrInfosFromP2pAddrs(as...)
}

// Advertise is a nop that defaults to PermanentAddrTTL.
func (as StaticAddrs) Advertise(_ context.Context, _ string, opt ...discovery.Option) (time.Duration, error) {
	opts := &discovery.Options{}
	if err := opts.Apply(opt...); err != nil {
		return 0, err
	}
	if opts.Ttl == 0 {
		opts.Ttl = ps.PermanentAddrTTL
	}
	return opts.Ttl, nil
}

// FindPeers converts the static addresses into AddrInfos
func (as StaticAddrs) FindPeers(_ context.Context, _ string, opt ...discovery.Option) (<-chan peer.AddrInfo, error) {
	opts := &discovery.Options{}
	if err := opts.Apply(opt...); err != nil {
		return nil, err
	}

	return staticChan(limited(opts, as)), nil
}

func limited(opts *discovery.Options, ps []peer.AddrInfo) []peer.AddrInfo {
	if opts.Limit > 0 && opts.Limit < len(ps) {
		ps = ps[:opts.Limit]
	}
	return ps
}

func staticChan(ps []peer.AddrInfo) chan peer.AddrInfo {
	ch := make(chan peer.AddrInfo, len(ps))
	for _, info := range ps {
		ch <- info
	}
	close(ch)
	return ch
}

func normalizeNS(ns string) string {
	// HACK:  libp2p's pubsub system prefixes topic names with the string "floodsub:",
	//        presumably to avoid collisions with DHT-based discovery.
	//
	// See:  https://github.com/libp2p/go-libp2p-pubsub/blob/v0.5.4/discovery.go#L322-L328
	return strings.TrimPrefix(ns, "floodsub:")
}

type errChanPool chan chan error

var chErrPool = make(errChanPool, 8)

func (pool errChanPool) Get() (cherr chan error) {
	select {
	case cherr = <-pool:
	default:
		cherr = make(chan error, 1)
	}

	return
}

func (pool errChanPool) Put(cherr chan error) {
	select {
	case <-cherr:
	default:
	}

	select {
	case pool <- cherr:
	default:
	}
}
