package net

import (
	"math/rand"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/lthibault/log"
	protoutil "github.com/wetware/casm/pkg/util/proto"
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
		// init the global PRNG?
		if globalRand == nil {
			globalRand = rand.New(rand.NewSource(time.Now().UnixNano()))
		}

		r = globalRand
	}
	ar := &atomicRand{r: r}

	return func(o *Overlay) {
		o.r = ar
		o.n.vtx.r = ar
	}
}

type atomicRand struct {
	r  *rand.Rand
	mu sync.Mutex
}

func (ar *atomicRand) Intn(n int) int {
	ar.mu.Lock()
	result := ar.r.Intn(n)
	ar.mu.Unlock()
	return result
}

func (ar *atomicRand) Shuffle(n int, swap func(i, j int)) {
	ar.mu.Lock()
	ar.r.Shuffle(n, swap)
	ar.mu.Unlock()
}

func withDefaults(opt []Option) []Option {
	return append([]Option{
		WithNamespace(""),
		WithLogger(Logger{}),
		WithRand(nil),
	}, append(opt, setLogFields, setProto)...)
}

// populate logger with fields set by options
func setLogFields(o *Overlay) {
	// Include only static properties in the base logger.  In particular,
	// do not set the 'edges' field.
	o.log = o.log.With(log.F{
		"type": "casm.net.overlay",
		"id":   o.h.ID(),
		"ns":   o.ns,
	})
}

// populate the overlay's protocol base path based on namespace to:
//
//	/casm/net/<version>/<namespace>
//
func setProto(o *Overlay) {
	o.proto = protoutil.Join(versionProto, protocol.ID(o.ns))
}
