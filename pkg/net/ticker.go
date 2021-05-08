package net

import (
	"sync"
	"time"
)

const Factor = 2

type backoff struct {
	current time.Duration
	min     time.Duration
	max     time.Duration

	isSteady bool
	mu       sync.Mutex
}

func newBackoff(min, max time.Duration) *backoff {
	return &backoff{min: min, max: max, current: min}
}

func (b *backoff) Jitter(time.Duration) time.Duration {
	b.mu.Lock()
	b.current = minDuration(time.Duration(b.current.Nanoseconds()*Factor), b.max)
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
