package mesh

import (
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/lthibault/log"
)

// Option type for Neighborhood.
type Option func(*Neighborhood)

// WithNamespace sets the namespace of the neighborhood.
//
// If ns == "", defaults to "casm".
func WithNamespace(ns string) Option {
	if ns == "" {
		ns = "casm"
	}

	return func(n *Neighborhood) {
		n.ns = ns
	}
}

// WithLogger sets the logger for the neighborhood.
//
// If l == nil, a default logger is used that provides
// human-readable output for log level INFO and above.
func WithLogger(l log.Logger) Option {
	if l == nil {
		l = log.New()
	}

	return func(n *Neighborhood) {
		n.log = l.With(n)
	}
}

// WithCardinality sets the maximum number of peers that
// can be in the neighborhood at any point in time.
//
// If k < 2, a default value of 5 is used.
func WithCardinality(k uint) Option {
	if k < 2 {
		k = 5
	}

	return func(n *Neighborhood) {
		n.slots = make(peer.IDSlice, 0, k)
	}
}

func withDefaults(opt []Option) []Option {
	return append([]Option{
		WithLogger(nil),
		WithNamespace(""),
		WithCardinality(5),
	}, opt...)
}
