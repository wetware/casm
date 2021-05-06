package net

import (
	"sync"
	"time"
)

type backoff struct {
	current time.Duration
	min     time.Duration
	max     time.Duration

	factor   int64
	isSteady bool
	mu       sync.Mutex
}

func newBackoff(min, max time.Duration, factor int64) *backoff {
	return &backoff{min: min, max: max, current: min, factor: factor, isSteady: false}
}

func (b *backoff) Jitter(time.Duration) time.Duration {
	b.mu.Lock()
	b.current = minDuration(time.Duration(b.current.Nanoseconds()*b.factor), b.max)
	b.mu.Unlock()
	return b.current
}

func (b *backoff) SetSteadyState(isSteady bool) {
	b.isSteady = isSteady
}

func (b *backoff) Reset() {
	if !b.isSteady {
		b.mu.Lock()
		b.current = b.min
		b.mu.Lock()
	}
}

func minDuration(d1, d2 time.Duration) time.Duration {
	if d1 < d2 {
		return d1
	}
	return d2
}
