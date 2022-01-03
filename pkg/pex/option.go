package pex

import (
	"time"

	ds "github.com/ipfs/go-datastore"
	nsds "github.com/ipfs/go-datastore/namespace"
	"github.com/ipfs/go-datastore/sync"
	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/lthibault/log"
	"go.uber.org/fx"
)

type Gossip struct {
	C int     // maximum View size
	S int     // swapping amount
	P int     // protection amount
	D float64 // retention decay probability
}

// Config supplies options to the dependency-injection framework.
type Config struct {
	fx.Out

	Log          log.Logger
	NewGossip    func(ns string) Gossip
	NewTick      func(ns string) time.Duration
	NewStore     func(ns string) ds.Batching
	Discovery    discovery.Discovery
	DiscoveryOpt []discovery.Option
}

func (c *Config) Apply(opt []Option) {
	for _, option := range withDefaults(opt) {
		option(c)
	}
}

type Option func(c *Config)

// WithLogger sets the logger for the peer exchange.
// If l == nil, a default logger is used.
func WithLogger(l log.Logger) Option {
	if l == nil {
		l = log.New()
	}

	return func(c *Config) {
		c.Log = l
	}
}

// WithDatastore sets the storage backend for gossip
// records.  If newStore == nil, a volatile storage backend
// is used.
//
// Note that s MUST be thread-safe.
func WithDatastore(newStore func(ns string) ds.Batching) Option {
	deafaultNewStore := func(ns string) ds.Batching {
		s := sync.MutexWrap(ds.NewMapDatastore())
		return nsds.Wrap(s, ds.NewKey("/casm/pex"))
	}

	if newStore == nil {
		newStore = deafaultNewStore
	}

	return func(c *Config) {
		c.NewStore = newStore
	}
}

// WithDiscovery sets the bootstrap discovery service
// for the PeX instance.  The supplied instance will
// be called with 'opt' whenever the PeeerExchange is
// unable to connect to peers in its cache.
func WithDiscovery(d discovery.Discovery, opt ...discovery.Option) Option {
	return func(c *Config) {
		c.DiscoveryOpt = opt
		c.Discovery = d
	}
}

// WithTick sets the interval between gossip rounds.
// A lower value of 'tick' improves cluster resiliency
// at the cost of increased bandwidth usage.
//
// If d == nil, a default value of 1m is used.  Users
// SHOULD NOT alter this value without good reason.
func WithTick(newTick func(ns string) time.Duration) Option {
	defaultNewTick := func(ns string) time.Duration {
		return time.Minute
	}
	if newTick == nil {
		newTick = defaultNewTick
	}

	return func(c *Config) {
		c.NewTick = newTick
	}
}

// WithGossip sets the parameters for gossiping:
// C, S, R and D. Check github.com/wetware/casm/specs/pex.md
// for more information on the meaining of each parameter.
//
// If n == nil, default values of {c=30, s=10, r=5, d=0.005} are used.
//
// Users SHOULD ensure all nodes in a given cluster have
// the same gossiping parameters.
func WithGossip(newGossip func(ns string) Gossip) Option {
	deafaultNewGossip := func(ns string) Gossip {
		return Gossip{30, 10, 5, 0.005}
	}

	if newGossip == nil {
		newGossip = deafaultNewGossip
	}

	return func(c *Config) {
		c.NewGossip = newGossip
	}
}

func withDefaults(opt []Option) []Option {
	return append([]Option{
		WithTick(nil),
		WithGossip(nil),
		WithLogger(nil),
		WithDatastore(nil),
		WithDiscovery(nil),
	}, opt...)
}
