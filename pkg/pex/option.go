package pex

import (
	"time"

	ds "github.com/ipfs/go-datastore"
	nsds "github.com/ipfs/go-datastore/namespace"
	"github.com/ipfs/go-datastore/sync"
	"github.com/libp2p/go-libp2p/core/discovery"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/lthibault/log"
	"github.com/wetware/casm/pkg/boot"
)

const (
	DefaultMaxView    = 32
	DefaultSwap       = 10
	DefaultProtect    = 5
	DefaultDecay      = 0.005
	DefaultTick       = time.Minute * 5
	DefaultTimeout    = time.Second * 30
	DefaultMaxMsgSize = 2048
)

var DefaultGossipConfig = GossipConfig{
	MaxView:    DefaultMaxView,
	Swap:       DefaultSwap,
	Protect:    DefaultProtect,
	Decay:      DefaultDecay,
	Tick:       DefaultTick,
	Timeout:    DefaultTimeout,
	MaxMsgSize: DefaultMaxMsgSize,
}

type Option func(px *PeerExchange)

// WithLogger sets the logger for the peer exchange.
// If l == nil, a default logger is used.
func WithLogger(l log.Logger) Option {
	if l == nil {
		l = log.New()
	}

	return func(px *PeerExchange) {
		px.log = l
	}
}

// WithDatastore sets the storage backend for gossip
// records.  If newStore == nil, a volatile storage backend
// is used.
//
// Note that store MUST be thread-safe.
func WithDatastore(store ds.Batching) Option {
	if store == nil {
		store = sync.MutexWrap(ds.NewMapDatastore())
		store = nsds.Wrap(store, ds.NewKey("/casm/pex"))
	}

	return func(px *PeerExchange) {
		px.store.Batching = store
	}
}

// WithDiscovery sets the bootstrap discovery service
// for the PeX instance.  The supplied instance will
// be called with 'opt' whenever the PeeerExchange is
// unable to connect to peers in its cache.
func WithDiscovery(d discovery.Discovery, opt ...discovery.Option) Option {
	if d == nil {
		d = boot.StaticAddrs(nil)
	}

	return func(px *PeerExchange) {
		px.disc.d = d
		px.disc.opt = opt
	}
}

// WithBootstrapPeers sets the bootstrap discovery service
// for the PeX instance to bootstrap with specific peers.
// It is a user-friendly way to set up the discovery service,
// and is exactly equivalent to:
//
//    WithDiscovery(boot.StaticAddrs{...})
//
//
// Namespaces will be bootstrapped using the supplied peers whenever
// the PeerExchange is unable to connect to peers in its cache.
func WithBootstrapPeers(peers ...peer.AddrInfo) Option {

	d := boot.StaticAddrs(peers)

	return func(px *PeerExchange) {
		if len(peers) > 0 {
			px.disc.d = d
		}
	}
}

// WithGossip sets the parameters for gossiping.
// See github.com/wetware/casm/specs/pex.md for details on the
// MaxView, Swap, Protect and Decay parameters.
//
// If newGossip == nil, the following default values are used
// for each namespace:
//
//    GossipConfig{
//        MaxView:    32,
//        Swap:       10,
//        Protect:    5,
//        Decay:      0.005,
//
//        Tick:       time.Minute * 5,
//        Timeout:    time.Second * 30,
//        MaxMsgSize: 2048,
//    }
//
// Users should exercise care when modifying the gossip params
// and ensure they fully understand the implications of their
// changes. Generally speaking, it is safe to increase MaxView.
// It is also reasonably safe to increase Decay by moderate
// amounts, in order to more aggressively expunge stale entries
// from cache.
//
// Users SHOULD ensure all nodes in a given namespace
// have identical GossipParam values.
func WithGossip(newGossip func(ns string) GossipConfig) Option {
	if newGossip == nil {
		newGossip = func(ns string) GossipConfig {
			return DefaultGossipConfig
		}
	}

	return func(px *PeerExchange) {
		px.newParams = newGossip
	}
}

func withDefaults(opt []Option) []Option {
	return append([]Option{
		WithLogger(nil),
		WithGossip(nil),
		WithDatastore(nil),
		WithDiscovery(nil),
	}, opt...)
}
