package stream_test

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"capnproto.org/go/capnp/v3"
	capnp_stream "capnproto.org/go/capnp/v3/std/capnp/stream"
	testing_api "github.com/wetware/casm/internal/api/testing"
	"github.com/wetware/casm/pkg/util/stream"
)

func init() {
	capnp.SetClientLeakFunc(func(msg string) {
		panic(msg)
	})
}

func TestMain(m *testing.M) {
	defer runtime.GC()
	m.Run()
}

func TestState(t *testing.T) {
	t.Parallel()
	t.Helper()

	t.Run("ImmediateWait", func(t *testing.T) {
		t.Parallel()

		/*
			Test that the stream does not block when it is terminated
			without any calls.

			Also checks that it's Open/Closed state is correct.
		*/

		server := &streamer{}
		client := testing_api.Streamer_ServerToClient(server)
		defer client.Release()

		s := stream.New(client.Recv)
		assert.True(t, s.Open(), "stream should be open")
		assert.NoError(t, s.Wait(), "stream should finish without error")
		assert.False(t, s.Open(), "stream should NOT be open")
	})

	t.Run("CallAndWait", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		/*
			Test normal stream operation by performing 10 calls,
			all of which succeed.
		*/

		server := sleepStreamer(time.Millisecond)
		client := testing_api.Streamer_ServerToClient(server)
		defer client.Release()

		s := stream.New(client.Recv)

		// stream 10 calls; each blocks for 1ms
		for i := 0; i < 10; i++ {
			s.Call(ctx, nil)
		}

		assert.True(t, s.Open(), "stream should be open")
		assert.NoError(t, s.Wait(), "stream should finish without error")
		assert.False(t, s.Open(), "stream should NOT be open")
	})

	t.Run("WaitInflight", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		/*
			Test that a stream gracefully terminates when there are in-
			flight requests.
		*/

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

		select {
		case <-time.After(time.Millisecond * 100):
			t.Error("failed to abort after 500ms")
		case err := <-cherr:
			assert.ErrorIs(t, err, context.Canceled)
			assert.False(t, s.Open(), "should be closed")
		}
	})

	t.Run("WaitRemoteError", func(t *testing.T) {
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

		// In order to break out of loops, it is important that
		// the context cancelation be detected *before* the call
		// to Wait()
		assert.Eventually(t, func() bool {
			return !s.Open()
		}, time.Millisecond*100, time.Millisecond*10,
			"should close before call to Wait()")

		assert.Error(t, s.Wait(),
			"Wait() should return error from server")
	})
}

func TestStream(t *testing.T) {
	t.Parallel()

	/*
		Test tracking of an actual stream of successful calls.
	*/

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server := counter(0)
	client := testing_api.Streamer_ServerToClient(&server)
	defer client.Release()

	s := stream.New(client.Recv)

	for i := 0; i < 100; i++ {
		require.True(t, s.Open(), "stream should be open")
		s.Call(ctx, nil)
	}

	assert.NoError(t, s.Wait(), "should succeed")
	assert.Equal(t, 100, int(server), "should process 100 calls")
}

func TestContextCanceled(t *testing.T) {
	t.Parallel()

	/*
		Test that a call with an expired context immediately causes the
		stream to stop processing subsequent calls.  This is crucial to
		prevent OOM failures due to a busy loop enqueuing calls.
	*/

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	server := &streamer{}
	client := testing_api.Streamer_ServerToClient(server)
	defer client.Release()

	s := stream.New(client.Recv)

	// make one successful call so that the receive-loop is
	// started.
	s.Call(ctx, nil)

	// The context was canceled *before* the call to Call(), so
	// it should always be detected *before* Call() returns.
	//
	// To avoid sudden, unbounded bursts in memory consumption, it
	// is crucial that this assertion pass.
	assert.False(t, s.Open(), "should close before Call() returns")

	err := s.Wait()
	assert.ErrorIs(t, err, context.Canceled, "error: %v", err)
}

func TestPlaceArgsFailure(t *testing.T) {
	t.Parallel()

	/*
		Test that a failure in PlaceArgs immediately causes s.Open() to
		return false.  This is crucial to prevent OOM failures due to a
		busy loop adding (failing) calls to the queue until the initial
		future is processed.
	*/

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server := &streamer{}
	client := testing_api.Streamer_ServerToClient(server)
	defer client.Release()

	s := stream.New(client.Recv)
	assert.True(t, s.Open(), "should initially be open")

	var errFail = errors.New("fail")
	s.Call(ctx, func(testing_api.Streamer_recv_Params) error {
		return errFail
	})

	assert.Eventually(t, func() bool {
		return !s.Open()
	}, time.Millisecond*100, time.Millisecond*10,
		"should close quickly after failed call to PlaceArgs")

	require.ErrorIs(t, s.Wait(), errFail, "should return error from PlaceArgs")
}

func TestIdempotentWait(t *testing.T) {
	t.Parallel()

	/*
		This is a regression test to ensure multiple calls to Wait()
		are idempotent, and do not panic.
	*/

	s := stream.New(nop) // not making calls, so nop ok
	err := s.Wait()
	require.NoError(t, err)

	assert.NotPanics(t, func() {
		err = s.Wait()
	}, "should not panic on second call to Wait()")

	assert.NoError(t, err, "should be idempotent")
}

func BenchmarkStream(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// We don't want to benchmark capnp; just the stream manager.
	// The focus is primarily on avoiding garbage from the underlying
	// linked-list.
	s := stream.New(nop)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		s.Call(ctx, nil)
	}

	if err := s.Wait(); err != nil {
		panic(err)
	}
}

var nopFuture = capnp_stream.StreamResult_Future{
	Future: capnp.ErrorAnswer(capnp.Method{
		InterfaceID:   0xef96789c0d60cd00,
		MethodID:      0,
		InterfaceName: "testing.capnp:Streamer",
		MethodName:    "recv",
	}, nil).Future(),
}

// CAUTION:  only use nop in benchmarks. It DOES NOT call PlaceArgs.
func nop(context.Context, func(testing_api.Streamer_recv_Params) error) (capnp_stream.StreamResult_Future, capnp.ReleaseFunc) {
	return nopFuture, func() {}
}

type streamer struct{ error }

func (s *streamer) Recv(ctx context.Context, err testing_api.Streamer_recv) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return s.error
}

type counter int

func (ctr *counter) Recv(context.Context, testing_api.Streamer_recv) error {
	*ctr++
	return nil
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
