package stream_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"capnproto.org/go/capnp/v3"
	capnp_stream "capnproto.org/go/capnp/v3/std/capnp/stream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	testing_api "github.com/wetware/casm/internal/api/testing"
	"github.com/wetware/casm/pkg/util/stream"
	"go.uber.org/atomic"
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

		s := stream.New[testing_api.Streamer_recv_Params](ctx)
		err := s.Call(client.Recv, nil)
		require.NoError(t, err, "streaming call should succeed")
	})

	t.Run("ContextExpired", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		server := &streamer{}
		client := testing_api.Streamer_ServerToClient(server)
		defer client.Release()

		s := stream.New[testing_api.Streamer_recv_Params](ctx)

		// make one successful call so that the receive-loop is
		// started.
		err := s.Call(client.Recv, nil)
		require.NoError(t, err, "streaming call should succeed")

		cancel()
		assert.ErrorIs(t, s.Wait(), context.Canceled,
			"should stop when context expires")
	})

	t.Run("HandlerError", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		server := &streamer{}
		client := testing_api.Streamer_ServerToClient(server)
		defer client.Release()

		s := stream.New[testing_api.Streamer_recv_Params](ctx)

		// make one successful call so that the receive-loop is
		// started.
		err := s.Call(client.Recv, nil)
		require.NoError(t, err, "streaming call should succeed")

		server.Store(errors.New("test"))

		assert.Eventually(t, func() bool {
			err := s.Call(client.Recv, nil)
			return errors.Is(err, server.Load())
		}, time.Second, time.Millisecond*100,
			"context expiration should close stream")
	})

	t.Run("DrainQueue", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ch := make(chan struct{}, 1)
		client := testing_api.Streamer_ServerToClient(sleeper(ch))
		defer client.Release()

		s := stream.New[testing_api.Streamer_recv_Params](ctx)
		var c ctr

		// enqueue 1s worth of calls
		for i := 0; i < 10; i++ {
			err := s.Call(c.wrap(client.Recv), nil)
			require.NoError(t, err, "streaming call should succeed")
		}

		require.NoError(t, s.Wait(), "should finish successfully")
		assert.True(t, c.Zero(), "should have released all references")
	})
}

type ctr struct{ atomic.Int32 }

type streamFunc func(context.Context, func(testing_api.Streamer_recv_Params) error) (capnp_stream.StreamResult_Future, capnp.ReleaseFunc)

func (c *ctr) wrap(f streamFunc) streamFunc {
	c.Inc()
	return func(ctx context.Context, args func(testing_api.Streamer_recv_Params) error) (capnp_stream.StreamResult_Future, capnp.ReleaseFunc) {
		f, release := f(ctx, args)
		return f, func() {
			c.Dec()
			release()
		}
	}
}

func (c *ctr) Zero() bool { return c.Int() == 0 }

func (c *ctr) Int() int32 { return c.Load() }

type streamer struct{ atomic.Error }

func (s *streamer) Recv(context.Context, testing_api.Streamer_recv) error {
	return s.Load()
}

type sleeper <-chan struct{}

func (s sleeper) Recv(ctx context.Context, _ testing_api.Streamer_recv) error {
	select {
	case <-time.After(time.Millisecond * 100):
		return nil
	case <-s:
		return errors.New("test")
	case <-ctx.Done():
		return ctx.Err()
	}

}
