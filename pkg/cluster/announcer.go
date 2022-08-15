package cluster

import (
	"context"
	"errors"
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

	mu sync.Mutex
	h  heartbeat
}

func newAnnouncer() *announcer {
	return &announcer{
		h: heartbeat{Heartbeat: pulse.NewHeartbeat()},
	}
}

func (a *announcer) Start() (err error) {
	a.cq = make(chan struct{})
	a.log = a.log.With(a)
	go a.tick()

	return
}

func (a *announcer) Close() error {
	close(a.cq)
	return nil
}

func (a *announcer) Loggable() map[string]any {
	fields := a.h.Loggable()
	fields["ns"] = a.t.String()
	return fields
}

func (a *announcer) tick() {
	a.log.Debug("started heartbeat loop")
	defer a.log.Debug("exited heartbeat loop")

	ticker := a.h.NewTicker()
	defer ticker.Stop()

	var (
		ctx context.Context = ctxutil.C(a.cq)
		b                   = a.h.NewBackoff()
	)

	for ctx.Err() == nil {
		if err := a.announce(ctx); err != nil {
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

func (a *announcer) announce(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	b, err := a.h.Next()
	if err != nil {
		return err
	}

	// Publish may return nil if the context shuts down.
	return a.t.Publish(ctx, b)
}

type heartbeat struct {
	pulse.Preparer
	pulse.Heartbeat
}

func (h heartbeat) NewTicker() *jitterbug.Ticker {
	return jitterbug.New(h.TTL()/2, jitterbug.Uniform{
		Min:    h.TTL() / 10,
		Source: rand.New(rand.NewSource(rand.Int63())),
	})
}

func (h heartbeat) NewBackoff() *loggableBackoff {
	return &loggableBackoff{backoff.Backoff{
		Factor: 2,
		Min:    h.TTL() / 10,
		Max:    time.Minute * 15,
		Jitter: true,
	}}
}

func (h *heartbeat) Next() ([]byte, error) {
	if err := h.prepare(); err != nil {
		return nil, err
	}

	return h.Message().MarshalPacked()
}

func (h *heartbeat) prepare() (err error) {
	if err := h.setHostname(); err == nil {
		h.setMeta()
	}

	return
}

func (h *heartbeat) setHostname() (err error) {
	if !h.hasHostname() {
		var name string
		if name, err = os.Hostname(); err == nil {
			err = h.SetHostname(name)
		}
	}

	return
}

func (h *heartbeat) hasHostname() bool {
	return h.HasHostname()
}

func (h *heartbeat) setMeta() {
	if h.Preparer != nil {
		h.Preparer.Prepare(h)
	}
}

func (h *heartbeat) SetMeta(meta map[string]string) error {
	panic("NOT IMPLEMENTED")
}

type loggableBackoff struct{ backoff.Backoff }

func (b *loggableBackoff) Loggable() map[string]interface{} {
	return map[string]interface{}{
		"attempt": int(b.Attempt()),
		"dur":     b.ForAttempt(b.Attempt()),
		"max_dur": b.Max,
	}
}
