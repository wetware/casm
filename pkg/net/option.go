package net

import (
	"math/rand"
	"time"

	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/lthibault/log"
)

var globalRand *rand.Rand

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

// WithRand sets the random number generator for the overlay.
//
// The most common use-case is reproducible testing.  Another
// common application is to seed the PRNG on the basis of the
// host's peer.ID, thereby resulting in deterministic behavior
// for individual hosts (which can be helpful when inspecting
// log messages) while preserving the stochastic properties of
// the cluster as a whole.
//
// If r == nil, a global *rand.Rand is used.
func WithRand(r *rand.Rand) Option {
	if r == nil {
		if globalRand == nil {
			globalRand = rand.New(rand.NewSource(time.Now().UnixNano()))
		}

		r = globalRand
	}

	return func(o *Overlay) {
		o.n.vtx.r = r
	}
}

// populate logger with fields set by options
func setLogFields(o *Overlay) { o.log = o.log.With(o) }

func withDefaults(opt []Option) []Option {
	return append([]Option{
		WithNamespace(""),
		WithLogger(Logger{}),
		WithRand(nil),
	}, append(opt, setLogFields)...)
}

/*
 * Options for Overlay.Sample.
 */

func SampleDepth(d uint8) discovery.Option {
	if d == 0 {
		d = 7
	}

	return func(opts *discovery.Options) error {
		if opts.Other == nil {
			opts.Other = map[interface{}]interface{}{}
		}

		opts.Other[keyDepth] = d
		return nil
	}
}

type sampleKey uint8

const (
	keyDepth sampleKey = iota
)
