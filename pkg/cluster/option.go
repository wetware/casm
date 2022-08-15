package cluster

import (
	"time"

	"github.com/lthibault/log"
	"github.com/wetware/casm/pkg/cluster/pulse"
	"github.com/wetware/casm/pkg/cluster/routing"
)

type Option func(*Node)

func WithNamespace(ns string) Option {
	if ns == "" {
		ns = "casm"
	}

	return func(m *Node) {
		m.ns = ns
	}
}

func WithTTL(d time.Duration) Option {
	if d <= 0 {
		d = time.Second * 10
	}

	if d < time.Millisecond {
		d = time.Millisecond
	}

	return func(m *Node) {
		m.a.SetTTL(d)
	}
}

func WithLogger(l log.Logger) Option {
	if l == nil {
		l = log.New()
	}

	return func(m *Node) {
		m.a.log = l
	}
}

func WithRoutingTable(t RoutingTable) Option {
	if t == nil {
		t = routing.New()
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
