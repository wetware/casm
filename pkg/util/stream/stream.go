package stream

import (
	"context"
	"sync"

	"capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/std/capnp/stream"
)

type Func[T ~capnp.StructKind] func(context.Context, func(T) error) (stream.StreamResult_Future, capnp.ReleaseFunc)

type Stream[T ~capnp.StructKind] struct {
	method Func[T]

	mu               sync.Mutex
	err              error
	started, closing bool
	inflight         queue
	signal, closed   chan struct{}
}

func New[T ~capnp.StructKind](method Func[T]) *Stream[T] {
	return &Stream[T]{
		signal: make(chan struct{}, 1),
		closed: make(chan struct{}),
		method: method,
	}
}

func (s *Stream[T]) Open() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return !s.closing
}

func (s *Stream[T]) Call(ctx context.Context, args func(T) error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.closing {
		// append to inflight queue
		s.inflight.Put(s.call(ctx, s.method, args))

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
	}
}

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
		for call := s.next(); call != nil; call = s.next() {
			s.await(call)
		}
	}
}

func (s *Stream[T]) next() *methodCall {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.inflight.Get()
}

func (s *Stream[T]) await(call *methodCall) {
	if err := call.Wait(); err != nil && s.err == nil {
		s.mu.Lock()
		defer s.mu.Unlock()

		s.err = err
		s.close()
	}
}

func (s *Stream[T]) call(ctx context.Context, f Func[T], args func(T) error) *methodCall {
	call := pool.Get()
	call.future, call.release = f(ctx, args)
	return call
}

// caller must hold the Stream mutex
func (s *Stream[T]) close() {
	if !s.started {
		close(s.closed)
	}

	if !s.closing {
		s.closing = true
		close(s.signal)
	}
}

type queue struct {
	first, last *methodCall
}

func (q *queue) Put(call *methodCall) {
	// empty?
	if q.first == nil {
		q.first = call
		q.last = call
	} else {
		q.last.next = call
		q.last = call
	}
}

func (q *queue) Get() *methodCall {
	call := q.first
	if call != nil {
		q.first = q.first.next
	}
	return call
}

type methodCall struct {
	future  stream.StreamResult_Future
	release capnp.ReleaseFunc
	next    *methodCall
}

func (call *methodCall) Wait() error {
	defer pool.Put(call)

	_, err := call.future.Struct()
	return err
}

var pool methodCallPool

type methodCallPool sync.Pool

func (p *methodCallPool) Get() *methodCall {
	if v := (*sync.Pool)(p).Get(); v != nil {
		return v.(*methodCall)
	}

	return new(methodCall)
}

func (p *methodCallPool) Put(call *methodCall) {
	call.release()
	call.next = nil
	call.release = nil
	call.future.Future = nil
	(*sync.Pool)(p).Put(call)
}
