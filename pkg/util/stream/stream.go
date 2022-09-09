package stream

import (
	"context"
	"sync"

	"capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/std/capnp/stream"
	"golang.org/x/sync/semaphore"
)

type Func[T ~capnp.StructKind] func(context.Context, func(T) error) (stream.StreamResult_Future, capnp.ReleaseFunc)

type Stream[T ~capnp.StructKind] struct {
	method   Func[T]
	inflight *queue
	mu       sync.Mutex
	closing  bool
	err      error
}

func New[T ~capnp.StructKind](method Func[T]) *Stream[T] {
	return &Stream[T]{
		method:   method,
		inflight: newQueue(),
	}
}

func (t *Stream[T]) Err() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.err
}

func (t *Stream[T]) Call(ctx context.Context, args func(T) error) {
	t.mu.Lock()

	if t.closing {
		t.mu.Unlock()
		return
	}

	go func() {
		ctx, cancel := context.WithCancel(ctx)
		f, release := t.method(ctx, args)
		defer release()
		defer cancel() // must happen before release()

		// We delay this call until the RPC is in flight to
		// preserve RPC ordering. We also want this call to
		// be as close as possible to inflight.Wait.
		t.mu.Unlock()

		// acquire the semaphore, then block until the call
		// is finished.
		if t.maybeFail(t.inflight.Wait(ctx)) {
			return
		}
		defer t.inflight.Done()

		_, err := f.Struct()
		t.maybeFail(err)
	}()
}

func (t *Stream[T]) Wait(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closing = true

	t.mu.Unlock()
	defer t.mu.Lock()

	// Don't release the semaphore.  Nobody else should
	// be calling it.
	if err := t.inflight.Wait(ctx); err != nil {
		return err
	}

	return t.err
}

func (t *Stream[T]) maybeFail(err error) (fail bool) {
	if err != nil {
		t.mu.Lock()
		defer t.mu.Unlock()

		if fail = t.err == nil; fail {
			t.err = err
			t.closing = true
		}
	}

	return
}

type queue semaphore.Weighted

func newQueue() *queue {
	return (*queue)(semaphore.NewWeighted(1))
}

// Wait returns when it is the caller's turn to proceed,
// or the context expires, whichever happens first.
//
// If err == nil, callers SHOULD call Done() to unblock
// the queue, when finished.
func (q *queue) Wait(ctx context.Context) error {
	return (*semaphore.Weighted)(q).Acquire(ctx, 1)
}

// Done signals that the caller is finished, and that the
// next thread can proceed.
func (q *queue) Done() {
	(*semaphore.Weighted)(q).Release(1)
}
