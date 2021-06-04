package pex

import (
	"math/rand"
	"sync"
	"time"
)

const (
	defaultViewSize = 10
	defaultTick     = time.Second
)

type Option func(o *PeerExchange)

func withDefaults(opt []Option) []Option {
	return append([]Option{WithNamespace(""), WithViewSize(defaultViewSize), WithTick(defaultTick)}, opt...)
}

// WithNamespace sets the namespace of the neighborhood.
//
func WithNamespace(ns string) Option {
	if ns == "" {
		ns = "casm/gossip" // TODO: decide what namespace to use
	}

	return func(o *PeerExchange) {
		o.ns = ns
	}
}

func WithViewSize(viewSize int) Option {
	return func(gspo *PeerExchange) {
		gspo.viewSize = viewSize
	}
}

func WithTick(tick time.Duration) Option {
	return func(gspo *PeerExchange) {
		gspo.tick = defaultTick
	}
}

var GlobalAtomicRand = &atomicRand{r: rand.New(rand.NewSource(time.Now().UnixNano()))}

type atomicRand struct {
	r  *rand.Rand
	mu sync.Mutex
}

func (ar *atomicRand) Intn(n int) int {
	ar.mu.Lock()
	defer ar.mu.Unlock()

	return ar.r.Intn(n)
}

func (ar *atomicRand) Shuffle(n int, swap func(i, j int)) {
	ar.mu.Lock()
	ar.r.Shuffle(n, swap)
	ar.mu.Unlock()
}
