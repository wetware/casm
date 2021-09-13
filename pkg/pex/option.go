package pex

import (
	"time"

	ds "github.com/ipfs/go-datastore"
	"github.com/lthibault/log"
	"go.uber.org/fx"
)

// Config supplies options to the dependency-injection framework.
type Config struct {
	fx.Out

	NS      string
	Log     log.Logger
	Tick    time.Duration
	MaxSize int
	Store   ds.Batching
}

func (c *Config) Apply(opt []Option) {
	for _, option := range withDefaults(opt) {
		option(c)
	}
}

type Option func(c *Config)

// WithNamespace sets the cluster namespace.
// If ns == "", the default "casm" is used.
func WithNamespace(ns string) Option {
	if ns == "" {
		ns = "casm"
	}
	return func(c *Config) {
		c.NS = ns
	}
}

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
func WithDatastore(s ds.Batching) Option {
	if s == nil {
		s = ds.NewMapDatastore()
	}

	return func(c *Config) {
		c.Store = s
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
func WithMaxViewSize(n uint) Option {
	if n == 0 {
		n = 32
	}

	return func(c *Config) {
		c.MaxSize = int(n)
	}
}

func withDefaults(opt []Option) []Option {
	return append([]Option{
		WithTick(-1),
		WithLogger(nil),
		WithNamespace(""),
		WithDatastore(nil),
		WithMaxViewSize(0),
	}, opt...)
}
