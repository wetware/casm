package stream

import (
	"context"
	"sync"

	"capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/std/capnp/stream"
	"github.com/lthibault/uq"
)

// Func is a streaming RPC method.
type Func[T ~capnp.StructKind] func(context.Context, func(T) error) (stream.StreamResult_Future, capnp.ReleaseFunc)

// Stream implements capnp streaming RPC calls.
//
// WARNING:  Stream makes use of an unbounded queue to track in-
// flight RPC calls.  Callers should carefully review the stream
// documentation before using Stream.  Use of capnp flow control
// is strongly RECOMMENDED.
type Stream[T ~capnp.StructKind] struct {
	method Func[T]

	mu               sync.Mutex
	err              error
	started, closing bool
	inflight         uq.Queue[promise]
	signal, closed   chan struct{}
}

// New RPC stream.  To avoid unbounded memory usage, callers SHOULD
// ensure a flowcontrol.FlowLimiter is assigned to method's client.
func New[T ~capnp.StructKind](method Func[T]) *Stream[T] {
	return &Stream[T]{
		method: method,
		signal: make(chan struct{}, 1),
		closed: make(chan struct{}),
	}
}

// Open returns true if the stream is open.  A stream is open until
// an invocation of Call() fails or Wait() returns, whichever comes
// fist.
func (s *Stream[T]) Open() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return !s.closing
}

// Call the streaming method.  Callers SHOULD pass the same context
// to each invocation of Call().   The stream is immediately closed
// if ctx.Err() != nil.
//
// After invoking Call(), callers MUST Wait() before discarding the
// stream.  Failure to do so will leak a goroutine.
func (s *Stream[T]) Call(ctx context.Context, args func(T) error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.closing {
		// issue the RPC call
		p := s.call(ctx, s.method, args)

		// append to inflight queue
		s.inflight.Push(p)

		// start background loop?
		if !s.started {
			s.started = true
			go s.loop()
		}

		// signal that a call is in flight
		select {
		case s.signal <- struct{}{}:
		default:
		}

		// stop accepting calls?
		if ctx.Err() != nil {
			s.close()
		}
	}
}

// Wait closes the stream and blocks until all in-flight RPC
// has terminated.  It returns the first error encountered.
func (s *Stream[T]) Wait() (err error) {
	s.mu.Lock()
	s.close()
	s.mu.Unlock()

	<-s.closed

	s.mu.Lock()
	err = s.err
	s.mu.Unlock()

	return

}

func (s *Stream[T]) loop() {
	defer close(s.closed)

	for range s.signal {
		for call, ok := s.next(); ok; call, ok = s.next() {
			s.await(call)
		}
	}
}

func (s *Stream[T]) next() (promise, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.inflight.Shift()
}

func (s *Stream[T]) await(call promise) {
	if err := call.Wait(); err != nil && s.err == nil {
		s.mu.Lock()
		defer s.mu.Unlock()

		s.err = err
		s.close()
	}
}

func (s *Stream[T]) call(ctx context.Context, f Func[T], args func(T) error) promise {
	future, release := f(ctx, args)
	return promise{
		future:  future,
		release: release,
	}
}

// caller must hold the Stream mutex
func (s *Stream[T]) close() {
	if !s.started {
		s.started = true
		close(s.closed)
	}

	if !s.closing {
		s.closing = true
		close(s.signal)
	}
}

type promise struct {
	future  stream.StreamResult_Future
	release capnp.ReleaseFunc
}

func (p promise) Wait() error {
	defer p.release()

	_, err := p.future.Struct()
	return err
}
