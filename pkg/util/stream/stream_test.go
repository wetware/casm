package stream_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	testing_api "github.com/wetware/casm/internal/api/testing"
	"github.com/wetware/casm/pkg/util/stream"
)

func TestStream(t *testing.T) {
	t.Parallel()
	t.Helper()

	t.Run("Succeed", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		server := &streamer{}
		client := testing_api.Streamer_ServerToClient(server)
		defer client.Release()

		s := stream.New(client.Recv)
		s.Call(ctx, nil)
	})

	t.Run("CallAndWait", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		server := sleepStreamer(time.Millisecond)
		client := testing_api.Streamer_ServerToClient(server)
		defer client.Release()

		s := stream.New(client.Recv)

		// stream 10 calls; each blocks for 1ms
		for i := 0; i < 10; i++ {
			s.Call(ctx, nil)
		}

		assert.NoError(t, s.Wait(), "should finish gracefully")
	})

	t.Run("AbortWait", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		server := sleepStreamer(time.Second)
		client := testing_api.Streamer_ServerToClient(server)
		defer client.Release()

		s := stream.New(client.Recv)

		s.Call(ctx, nil)

		cherr := make(chan error, 1)
		go func() {
			cherr <- s.Wait()
		}()

		cancel()

		assert.Eventually(t, func() bool {
			return !s.Open()
		}, time.Millisecond*100, time.Millisecond*10,
			"call to Wait() should signal stream close")

		select {
		case <-time.After(time.Millisecond * 500):
			t.Error("failed to abort after 500ms")
		case err := <-cherr:
			require.ErrorIs(t, err, context.Canceled)
		}
	})

	t.Run("ContextExpired", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		server := &streamer{}
		client := testing_api.Streamer_ServerToClient(server)
		defer client.Release()

		s := stream.New(client.Recv)

		// make one successful call so that the receive-loop is
		// started.
		s.Call(ctx, nil)

		cancel()
		assert.ErrorIs(t, s.Wait(), context.Canceled,
			"Wait() should return context error")
	})

	t.Run("HandlerError", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		server := &streamer{error: errors.New("test")}
		client := testing_api.Streamer_ServerToClient(server)
		defer client.Release()

		s := stream.New(client.Recv)

		// Make a call so that the stream's receive-loop receives the
		// server error, which should cause the stream to abort.
		s.Call(ctx, nil)

		assert.Error(t, s.Wait(),
			"Wait() should return error from server")
	})
}

type streamer struct{ error }

func (s *streamer) Recv(context.Context, testing_api.Streamer_recv) error {
	return s.error
}

type sleepStreamer time.Duration

func (s sleepStreamer) Recv(ctx context.Context, _ testing_api.Streamer_recv) error {
	select {
	case <-time.After(time.Duration(s)):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
