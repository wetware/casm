package cluster

import (
	"time"
)

// Option type for Cluster.
type Option func(*Cluster)

// WithNamespace sets the namespace for the cluster.
// If ns == "", a default namespace of "casm" is used.
func WithNamespace(ns string) Option {
	if ns == "" {
		ns = "casm"
	}

	return func(c *Cluster) {
		c.ns = ns
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

	return func(c *Cluster) {
		c.ttl = d
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

	return func(c *Cluster) {
		c.hook = f
	}
}

func withDefault(opt []Option) []Option {
	return append([]Option{
		WithNamespace(""),
		WithTTL(0),
		WithHook(nil),
	}, opt...)
}
