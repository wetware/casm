package cluster

import (
	"time"

	"github.com/lthibault/log"
	"github.com/wetware/casm/pkg/cluster/pulse"
	"github.com/wetware/casm/pkg/cluster/routing"
)

type Option func(*Node)

// WithNamespace sets the namespace for the cluster.
// If ns == "", defaults to "casm".
func WithNamespace(ns string) Option {
	if ns == "" {
		ns = "casm"
	}

	return func(m *Node) {
		m.ns = ns
	}
}

// WithTTL sets the time-to-live for the cluster node.
// If d <= 0, defaults to 10s. Take care when changing
// the TTL.  Users SHOULD NOT change the TTL without a
// full understanding of the implications.
func WithTTL(d time.Duration) Option {
	if d <= 0 {
		d = pulse.DefaultTTL
	}

	if d < time.Millisecond {
		d = time.Millisecond
	}

	return func(m *Node) {
		m.a.SetTTL(d)
	}
}

// WithLogger sets the cluster node's logger.
// If l == nil, a default logger is used, which logs all
// events at or above INFO level.
func WithLogger(l log.Logger) Option {
	if l == nil {
		l = log.New()
	}

	return func(m *Node) {
		m.a.log = l
	}
}

// WithRoutingTable sets the routing table for the cluster node.
// If t == nil, a default in-memory implementation is used.  The
// main use for this option is to support extremely large cluster
// sizes where the entire routing table does not fit into memory.
func WithRoutingTable(t RoutingTable) Option {
	if t == nil {
		t = routing.New(time.Now())
	}

	return func(m *Node) {
		m.rt = t
	}
}

// WithMeta specifies the heartbeat metadata by means of a Preparer,
// which is responsible for assigning metadata to the heartbeat. The
// preparer is called prior to each heartbeat emission.
//
// If meta == nil, no metadata is assigned.
func WithMeta(meta pulse.Preparer) Option {
	return func(n *Node) {
		n.a.Preparer = meta
	}
}

func withDefault(opt []Option) []Option {
	return append([]Option{
		WithNamespace(""),
		WithTTL(-1),
		WithLogger(nil),
		WithRoutingTable(nil),
		WithMeta(nil),
	}, opt...)
}
