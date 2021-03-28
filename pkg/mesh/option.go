package mesh

import (
	"context"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/lthibault/log"
)

// Option type for Neighborhood.
type Option func(*Neighborhood)

// WithContext sets the neighborhood's root context.
//
// If ctx == nil, context.Background() is used.
func WithContext(ctx context.Context) Option {
	if ctx == nil {
		ctx = context.Background()
	}

	return func(n *Neighborhood) {
		n.ctx, n.cancel = context.WithCancel(ctx)
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

// WithCallback sets the callback that is invoked when a
// neighbor (dis)connects.
//
// If f == nil, the callback is a nop.
func WithCallback(f func(Event, peer.ID)) Option {
	if f == nil {
		f = func(Event, peer.ID) {}
	}

	return func(n *Neighborhood) {
		n.cb = f
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
		n.ns = make(peer.IDSlice, 0, k)
	}
}

func withDefaults(opt []Option) []Option {
	return append([]Option{
		WithCallback(nil),
		WithCardinality(5),
		WithContext(context.Background()),
	}, opt...)
}
