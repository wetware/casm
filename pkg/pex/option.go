package pex

import (
	ds "github.com/ipfs/go-datastore"
	nsds "github.com/ipfs/go-datastore/namespace"
	"github.com/ipfs/go-datastore/sync"
	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/lthibault/log"
	"github.com/wetware/casm/pkg/boot"
)

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
		px.disc = d
		px.discOpt = opt
	}
}

// // WithTick sets the interval between gossip rounds.
// // A lower value of 'tick' improves cluster resiliency
// // at the cost of increased bandwidth usage.
// //
// // If d == nil, a default value of 1m is used.  Users
// // SHOULD NOT alter this value without good reason.
// func WithTick(newTick func(ns string) time.Duration) Option {
// 	if newTick == nil {
// 		newTick = func(ns string) time.Duration {
// 			return time.Minute
// 		}
// 	}

// 	return func(px *PeerExchange) {
// 		px.newTick = newTick
// 	}
// }

// // WithGossip sets the parameters for gossiping:
// // C, S, R and D. Check github.com/wetware/casm/specs/pex.md
// // for more information on the meaining of each parameter.
// //
// // If n == nil, default values of {c=30, s=10, r=5, d=0.005} are used.
// //
// // Users SHOULD ensure all nodes in a given cluster have
// // the same gossiping parameters.
// func WithGossip(newGossip func(ns string) GossipParam) Option {
// 	deafaultNewGossip := func(ns string) GossipParam {
// 		return GossipParam{30, 10, 5, 0.005}
// 	}

// 	if newGossip == nil {
// 		newGossip = deafaultNewGossip
// 	}

// 	return func(px *PeerExchange) {
// 		px.newGossip = newGossip
// 	}
// }

func withDefaults(opt []Option) []Option {
	return append([]Option{
		WithLogger(nil),
		WithDatastore(nil),
		// WithTick(nil),
		// WithGossip(nil),
		WithDiscovery(nil),
	}, opt...)
}

/*
	Discovery Options
*/

type (
	keyGossipParam struct{}
)

func gossipParams(opts *discovery.Options) GossipParam {
	ps := GossipParam{C: 30, S: 10, P: 5, D: 0.005}

	if v := opts.Other[keyGossipParam{}]; v != nil {
		ps = v.(GossipParam)
	}

	return ps
}

// WithGossipParam is consumed by PeX.Advertise.  If the namespace does
// not exist, it will be created with gossip parameters specified by ps.
// If the namespace already exists, WithGossipParam is ignored.
func WithGossipParam(ps GossipParam) discovery.Option {
	return func(opts *discovery.Options) error {
		if opts.Other == nil {
			opts.Other = make(map[interface{}]interface{}, 1)
		}

		opts.Other[keyGossipParam{}] = ps
		return nil
	}
}
