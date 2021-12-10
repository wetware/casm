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

type GossipParams struct {
	c int     // maximum View size
	s int     // swapping amount
	r int     // retention amount
	d float64 // retention decay probability
}

// Config supplies options to the dependency-injection framework.
type Config struct {
	fx.Out

	Log          log.Logger
	Gossip       GossipParams
	Tick         time.Duration
	Store        ds.Batching
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
// records.  If s == nil, a volatile storage backend
// is used.
//
// Note that s MUST be thread-safe.
func WithDatastore(s ds.Batching) Option {
	if s == nil {
		s = sync.MutexWrap(ds.NewMapDatastore())
	}

	return func(c *Config) {
		c.Store = nsds.Wrap(s, ds.NewKey("/casm/pex"))
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
// A lower value of 'd' improves cluster resiliency
// at the cost of increased bandwidth usage.
//
// If d <= 0, a default value of 1m is used.  Users
// SHOULD NOT alter this value without good reason.
func WithTick(d time.Duration) Option {
	if d <= 0 {
		d = time.Minute
	}

	return func(c *Config) {
		c.Tick = d
	}
}

// WithMaxViewSize sets the maximum size of the view.
//
// If n == 0, a default value of 32 is used.
//
// Users SHOULD ensure all nodes in a given cluster have
// the same maximum view size.
func WithGossipParams(gossip GossipParams) Option {
	if gossip.c <= 0 {
		gossip.c = 32
	}
	if gossip.s < 0 {
		gossip.s = (gossip.c / 2) * (2 / 3)
	}
	if gossip.r < 0 {
		gossip.r = (gossip.c / 2) * (1 / 3)
	}
	if gossip.d < 0 {
		gossip.d = 0.005
	}

	return func(c *Config) {
		c.Gossip = gossip
	}
}

func withDefaults(opt []Option) []Option {
	return append([]Option{
		WithTick(time.Minute),
		WithGossipParams(GossipParams{32, 10, 5, 0.005}),
		WithLogger(nil),
		WithDatastore(nil),
		WithDiscovery(nil),
	}, opt...)
}
