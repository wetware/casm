package stream

import (
	"context"
	"sync"

	"capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/exp/mpsc"
	"capnproto.org/go/capnp/v3/std/capnp/stream"
)

// Stream tracks inflight futures from streaming capabilities.
type Stream struct {
	ctx context.Context
	mu  sync.Mutex
	err error
	q   *mpsc.Queue[inflight]
}

// New Stream.  In-flight requests are automatically released when
// the context expires.  Callers SHOULD pass a context that expires
// when the stream is closed.
func New(ctx context.Context) *Stream {
	return &Stream{ctx: ctx}
}

type inflight struct {
	Future  stream.StreamResult_Future
	Release capnp.ReleaseFunc
}

func (i inflight) Await(ctx context.Context) error {
	defer i.Release()

	select {
	case <-struct{ *capnp.Future }(i.Future).Done():
		_, err := struct{ *capnp.Future }(i.Future).Struct()
		return err

	case <-ctx.Done():
		return ctx.Err()
	}
}

// Track the in-flight request. Tracked requests are automatically
// released.  Calls to Track will return non-nil errors iff (a) an
// in-flight request returned an error, or (b) the context expired.
func (s *Stream) Track(f stream.StreamResult_Future, r capnp.ReleaseFunc) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.err != nil {
		r()
		return s.err
	}

	if s.q == nil {
		s.q = mpsc.New[inflight]()
		go s.recv()
	}

	s.q.Send(inflight{
		Future:  f,
		Release: r,
	})

	return nil
}

func (s *Stream) recv() {
	var (
		i   inflight
		err error
	)

	for {
		if i, err = s.q.Recv(s.ctx); err != nil {
			break
		}

		if err = i.Await(s.ctx); err != nil {
			break
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.err = err

	// drain and release
	for i, ok := s.q.TryRecv(); ok; i, ok = s.q.TryRecv() {
		i.Release()
	}
}
