package stream_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"capnproto.org/go/capnp/v3"
	capnp_stream "capnproto.org/go/capnp/v3/std/capnp/stream"
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

		s := stream.New(ctx)
		err := s.Track(client.Recv(ctx, nil))
		require.NoError(t, err, "streaming call should succeed")
	})

	t.Run("ContextExpired", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		server := &streamer{}
		client := testing_api.Streamer_ServerToClient(server)
		defer client.Release()

		s := stream.New(ctx)

		// make one successful call so that the receive-loop is
		// started.
		err := s.Track(client.Recv(context.Background(), nil))
		require.NoError(t, err, "streaming call should succeed")

		cancel()

		assert.Eventually(t, func() bool {
			err := s.Track(client.Recv(ctx, nil))
			return errors.Is(err, context.Canceled)
		}, time.Second, time.Millisecond*100,
			"context expiration should close stream")
	})

	t.Run("HandlerError", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		server := &streamer{}
		client := testing_api.Streamer_ServerToClient(server)
		defer client.Release()

		s := stream.New(ctx)

		// make one successful call so that the receive-loop is
		// started.
		err := s.Track(client.Recv(ctx, nil))
		require.NoError(t, err, "streaming call should succeed")

		server.error = errors.New("test")

		assert.Eventually(t, func() bool {
			err := s.Track(client.Recv(ctx, nil))
			return errors.Is(err, server.error)
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

		s := stream.New(ctx)
		var c ctr

		// enqueue 1s worth of calls
		for i := 0; i < 10; i++ {
			err := s.Track(c.wrap(client.Recv(ctx, nil)))
			require.NoError(t, err, "streaming call should succeed")
		}

		// Next in-flight request will fail.  Subsequent calls are
		// unaffected.
		ch <- struct{}{}

		require.Eventually(t, func() bool {
			isZero := c.Zero()
			return isZero
		}, time.Second, time.Millisecond*100,
			"queue should be drained (%d outstanding)", c.Int())
	})
}

type ctr int32

func (c *ctr) wrap(f capnp_stream.StreamResult_Future, r capnp.ReleaseFunc) (capnp_stream.StreamResult_Future, capnp.ReleaseFunc) {
	atomic.AddInt32((*int32)(c), 1)
	return f, func() {
		atomic.AddInt32((*int32)(c), -1)
		r()
	}
}

func (c *ctr) Zero() bool { return c.Int() == 0 }

func (c *ctr) Int() int32 { return atomic.LoadInt32((*int32)(c)) }

type streamer struct{ error }

func (s streamer) Recv(context.Context, testing_api.Streamer_recv) error {
	return s.error
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
