package stream

import (
	"context"
	"sync"

	"capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/exp/mpsc"
	"capnproto.org/go/capnp/v3/std/capnp/stream"
	casm "github.com/wetware/casm/pkg"
	"go.uber.org/atomic"
)

// Stream tracks inflight futures from streaming capabilities.
type Stream[T ~capnp.StructKind] struct {
	mu   sync.Mutex
	once sync.Once

	ctx      context.Context
	err      error
	done     chan struct{}
	q        *mpsc.Queue[inflightCall]
	finish   atomic.Bool
	inflight atomic.Uint32
}

// New Stream.  In-flight requests are automatically released when
// the context expires.  Callers SHOULD pass a context that expires
// when the stream is closed.
func New[T ~capnp.StructKind](ctx context.Context) *Stream[T] {
	s := &Stream[T]{q: mpsc.New[inflightCall]()}
	s.Reset(ctx)
	return s
}

func (s *Stream[T]) Reset(ctx context.Context) {
	s.inflight.Store(0)
	s.finish.Store(false)
	s.done = make(chan struct{})
	s.once = sync.Once{}
	s.ctx = ctx
	s.err = nil
}

// Call the stream method and track the in-flight request. Tracked
// requests are automatically released when they complete or fail.
// Call returns a nil error unless (a) its context has expired, or
// (b) a previous request has failed.
func (s *Stream[T]) Call(f func(context.Context, func(T) error) (stream.StreamResult_Future, capnp.ReleaseFunc), args func(T) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.err == nil && !s.finish.Load() {
		defer s.inflight.Inc()
		s.once.Do(func() { go s.recv() })
		s.q.Send(inflight(f(s.ctx, args)))
	}

	return s.err
}

// Wait for inflight requests to complete.  Callers MUST NOT call
// the Call() method after calling Wait(). Wait returns after all
// requests have finished or the context expires, whichever comes
// first.
func (s *Stream[T]) Wait() error {
	s.once.Do(func() { go s.recv() })
	s.finish.Store(true)

	select {
	case <-s.ctx.Done():
		return s.ctx.Err()

	case <-s.done:
		return s.err
	}
}

func (s *Stream[T]) recv() {
	defer close(s.done)

	var (
		i   inflightCall
		err error
	)

	for ; !s.finish.Load() && s.inflight.Load() != 0; s.inflight.Dec() {
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
		s.inflight.Dec()
		i.Release()
	}
}

type inflightCall struct {
	Future  casm.Future
	Release capnp.ReleaseFunc
}

func inflight(f stream.StreamResult_Future, r capnp.ReleaseFunc) inflightCall {
	return inflightCall{
		Future:  casm.Future(f),
		Release: r,
	}
}

func (i inflightCall) Await(ctx context.Context) error {
	defer i.Release()
	return i.Future.Await(ctx)
}
