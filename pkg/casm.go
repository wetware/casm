package casm

import (
	"sync/atomic"

	"github.com/libp2p/go-libp2p-core/record"
)

type Register atomic.Value

func New(e *record.Envelope) *Register {
	r := new(Register)
	r.Store(e)
	return r
}

func (r *Register) Load() (*record.Envelope, bool) {
	e, ok := (*atomic.Value)(r).Load().(*record.Envelope)
	return e, ok
}

func (r *Register) Store(e *record.Envelope) { (*atomic.Value)(r).Store(e) }

func (r *Register) CompareAndSwap(old, new *record.Envelope) bool {
	return (*atomic.Value)(r).CompareAndSwap(old, new)
}
