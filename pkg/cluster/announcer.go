package cluster

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"sync"
	"time"

	"github.com/jpillora/backoff"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/lthibault/jitterbug/v2"
	"github.com/lthibault/log"
	ctxutil "github.com/lthibault/util/ctx"

	"github.com/wetware/casm/pkg/cluster/pulse"
)

type announcer struct {
	log log.Logger
	cq  chan struct{}
	t   *pubsub.Topic
	*emitter
}

func newAnnouncer() *announcer {
	return &announcer{
		cq:      make(chan struct{}),
		emitter: newEmitter(),
	}
}

func (a *announcer) Start() (err error) {
	a.log = a.log.With(a)
	go a.tick()

	return
}

func (a *announcer) Close() error {
	close(a.cq)
	return nil
}

func (a *announcer) Loggable() map[string]any {
	fields := a.emitter.Loggable()
	fields["ns"] = a.t.String()
	return fields
}

func (a *announcer) tick() {
	a.log.Debug("started heartbeat loop")
	defer a.log.Debug("exited heartbeat loop")

	ticker := a.NewTicker()
	defer ticker.Stop()

	var (
		ctx context.Context = ctxutil.C(a.cq)
		b                   = a.NewBackoff()
	)

	for ctx.Err() == nil {
		if err := a.Emit(ctx, a.t); err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}

			a.log.WithError(err).
				With(b).
				Warn("failed to emit heartbeat")

			select {
			case <-time.After(b.Duration()):
				a.log.With(b).Info("resuming")
				continue

			case <-ctx.Done():
				return
			}
		}

		a.log.Trace("heartbeat emitted")
		b.Reset()

		select {
		case <-ticker.C:
		case <-ctx.Done():
		}
	}
}

type emitter struct {
	mu sync.Mutex
	pulse.Preparer
	setter
}

func newEmitter() *emitter {
	return &emitter{
		setter: setter{pulse.NewHeartbeat()},
	}
}

type publisher interface {
	Publish(context.Context, []byte, ...pubsub.PubOpt) error
}

func (e *emitter) Emit(ctx context.Context, p publisher, opt ...pubsub.PubOpt) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	b, err := e.next()
	if err == nil {
		err = p.Publish(ctx, b, opt...)
	}

	return err
}

func (e *emitter) NewTicker() *jitterbug.Ticker {
	return jitterbug.New(e.TTL()/2, jitterbug.Uniform{
		Min:    e.TTL() / 10,
		Source: rand.New(rand.NewSource(rand.Int63())),
	})
}

func (e *emitter) NewBackoff() *loggableBackoff {
	return &loggableBackoff{backoff.Backoff{
		Factor: 2,
		Min:    e.TTL() / 10,
		Max:    time.Minute * 15,
		Jitter: true,
	}}
}

func (e *emitter) next() ([]byte, error) {
	if err := e.prepare(); err != nil {
		return nil, err
	}

	return e.Message().MarshalPacked()
}

func (e *emitter) prepare() (err error) {
	if err = e.setHostname(); err == nil {
		err = e.setMeta()
	}

	return
}

func (e *emitter) setHostname() (err error) {
	if !e.HasHostname() {
		var name string
		if name, err = os.Hostname(); err == nil {
			err = e.SetHostname(name)
		}
	}

	return
}

func (e *emitter) setMeta() (err error) {
	if e.Preparer != nil {
		err = e.Preparer.Prepare(&e.setter)
	}

	return
}

type setter struct{ pulse.Heartbeat }

func (s setter) SetMeta(meta map[string]string) error {
	size := len(meta)
	fields, err := s.NewMeta(int32(size))
	if err != nil {
		return err
	}

	for key, value := range meta {
		size--

		err = fields.Set(size, fmt.Sprintf("%s=%s", key, value))
		if err != nil {
			break
		}
	}

	return err
}

type loggableBackoff struct{ backoff.Backoff }

func (b *loggableBackoff) Loggable() map[string]interface{} {
	return map[string]interface{}{
		"attempt": int(b.Attempt()),
		"dur":     b.ForAttempt(b.Attempt()),
		"max_dur": b.Max,
	}
}
