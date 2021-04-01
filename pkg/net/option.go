package net

import (
	"context"
	"errors"
	"time"

	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/lthibault/log"
)

// Option for overlay network.
type Option func(o *Overlay)

// WithNamespace sets the namespace of the neighborhood.
//
// If ns == "", defaults to "casm".
func WithNamespace(ns string) Option {
	if ns == "" {
		ns = "casm"
	}

	return func(o *Overlay) {
		o.ns = ns
	}
}

// WithLogger sets the network logger.
//
// Passing a zero-value Logger is valid, and will configure a default
// logger that outputs all events of INFO-level and higher to stderr
// in a human-readable text format.
func WithLogger(l Logger) Option {
	if l.Logger == nil {
		l.Logger = log.New()
	}

	return func(o *Overlay) {
		o.log = l
	}
}

func withDefaults(opt []Option) []Option {
	// ensure the logger has up-to-date fields.
	opt = append(opt, func(o *Overlay) {
		o.log = o.log.With(o)
		o.n.log = o.log.WithField("type", "casm.net.neighborhood")
	})

	return append([]Option{
		WithLogger(Logger{}),
	}, opt...)
}

// Options for Overlay.Sample.

func SampleDepth(d uint8) discovery.Option {
	if d == 0 {
		d = 7
	}

	return func(opts *discovery.Options) error {
		if opts.Other == nil {
			opts.Other = map[interface{}]interface{}{}
		}

		opts.Other[depthOptKey{}] = d
		return nil
	}
}

func sampleTimeout(ctx context.Context) discovery.Option {
	return func(opts *discovery.Options) error {
		if opts.Ttl != 0 {
			return errors.New("TTL not supported")
		}

		opts.Ttl = time.Second * 30 // default

		if dl, ok := ctx.Deadline(); ok {
			opts.Ttl = time.Until(dl)
		}

		return nil
	}
}

func defaultSampleOpt(ctx context.Context, opt []discovery.Option) []discovery.Option {
	return append([]discovery.Option{
		discovery.Limit(1),
		sampleTimeout(ctx),
		SampleDepth(0),
	}, opt...)
}

type (
	depthOptKey struct{}
)
