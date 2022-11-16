package debug_test

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/wetware/casm/pkg/debug"
)

func TestSampler(t *testing.T) {
	t.Parallel()
	t.Helper()

	t.Run("NoStrategy", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		err := debug.SamplingServer{}.
			Sampler().
			Sample(context.Background(), &buf, 0)
		assert.ErrorIs(t, err, debug.ErrNoStrategy,
			"should report missing strategy")
		assert.Empty(t, buf.Bytes(), "should not write data")
	})

	t.Run("SampleDuration", func(t *testing.T) {
		t.Parallel()

		var (
			want = time.Millisecond * 100
			buf  bytes.Buffer
		)

		strategy := newMockStrategy(t)

		server := debug.SamplingServer{
			Strategy: strategy,
		}

		err := server.Sampler().Sample(context.Background(), &buf, want)
		assert.NoError(t, err, "sampling should succeed")
		assert.NotEmpty(t, buf.Bytes(), "should write samples")

		duration := strategy.Duration().Round(want)
		assert.Equal(t, want, duration, "should run for %s", want)
	})

	t.Run("Abort", func(t *testing.T) {
		t.Parallel()

		var (
			want = time.Millisecond * 50
			dur  = time.Millisecond * 100
			buf  bytes.Buffer
		)

		strategy := newMockStrategy(t)

		server := debug.SamplingServer{
			Strategy: strategy,
		}

		ctx, cancel := context.WithTimeout(context.Background(), want)
		defer cancel()

		err := server.Sampler().Sample(ctx, &buf, dur)
		assert.Error(t, err, "should abort sampling")
		assert.NotEmpty(t, buf.Bytes(), "should write samples")

		duration := strategy.Duration().Round(want)
		assert.Equal(t, want, duration, "should run for %s", want)
	})
}

type mockStrategy struct {
	t                 *testing.T
	t0, t1            time.Time
	stopping, stopped chan struct{}
}

func newMockStrategy(t *testing.T) *mockStrategy {
	return &mockStrategy{
		t:        t,
		stopping: make(chan struct{}),
		stopped:  make(chan struct{}),
	}
}

func (s *mockStrategy) Start(w io.Writer) error {
	s.t0 = time.Now()

	go func() {
		defer close(s.stopped)

		ticker := time.NewTicker(time.Millisecond * 10)
		defer ticker.Stop()

		s.t.Log("started")
		defer s.t.Log("stopped")

		for {
			select {
			case s.t1 = <-ticker.C:
				_, _ = w.Write([]byte{0xFF})
			case <-s.stopping:
				return
			}
		}
	}()

	return nil
}

func (s *mockStrategy) Stop() {
	s.t.Log("stopping")
	close(s.stopping)
	<-s.stopped
}

func (s *mockStrategy) Duration() time.Duration {
	return s.t1.Sub(s.t0)
}
