package cluster

import (
	"time"

	"github.com/lthibault/log"
	"go.uber.org/fx"
)

// Config provides options to the dependency-injection
// framework.
type Config struct {
	fx.Out

	NS   string
	Log  log.Logger
	TTL  time.Duration
	Hook Hook
}

// Apply options to 'c', populating its fields.
func (c *Config) Apply(opt []Option) {
	for _, option := range withDefault(opt) {
		option(c)
	}
}

// Option type for Cluster.
type Option func(*Config)

// WithNamespace sets the namespace for the cluster.
// If ns == "", a default namespace of "casm" is used.
func WithNamespace(ns string) Option {
	if ns == "" {
		ns = "casm"
	}

	return func(c *Config) {
		c.NS = ns
	}
}

// WithLogger sets the logger for the cluster model.
// If l == nil, a default logger is used.
func WithLogger(l log.Logger) Option {
	if l == nil {
		l = log.New()
	}

	return func(c *Config) {
		c.Log = l
	}
}

// WithTTL specifies the TTL for the heartbeat protocol.
// If d == 0, a default value of 6 seconds is used, which
// suitable for most applications.
//
// The most common reason to adjust the TTL is in testing,
// where it may be desirable to reduce the time needed for
// peers to become mutually aware.
func WithTTL(d time.Duration) Option {
	if d == 0 {
		d = time.Second * 6
	}

	return func(c *Config) {
		c.TTL = d
	}
}

// WithHook sets a heartbeat hook, which allows heartbeats
// to be modified immediately prior to broadcast.
//
// Users can use hooks to set metadata, modify the TTL, or
// perform arbitrary computation.  Users should take care
// not to block, as delaying heartbeats can cause peers to
// drop the local node from their routing tables.
//
// Callers should also be aware that the heartbeat is ALWAYS
// broadcast when 'f' returns.  f MUST NOT leave heartbeats
// in an invalid state.  Clean up after yourself!
//
// Passing f == nil removes the hook.
func WithHook(f func(Heartbeat)) Option {
	if f == nil {
		f = func(Heartbeat) {}
	}

	return func(c *Config) {
		c.Hook = f
	}
}

func withDefault(opt []Option) []Option {
	return append([]Option{
		WithNamespace(""),
		WithLogger(nil),
		WithTTL(0),
		WithHook(nil),
	}, opt...)
}
