package cluster

import (
	"context"
	"errors"
	"math/rand"
	"sync"
	"time"

	"capnproto.org/go/capnp/v3"
	"github.com/jpillora/backoff"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/lthibault/jitterbug/v2"
	"github.com/lthibault/log"
	ctxutil "github.com/lthibault/util/ctx"
	"github.com/wetware/casm/pkg/cluster/pulse"
)

type announcer struct {
	cq    chan struct{}
	ready pubsub.RouterReady

	log log.Logger
	ttl time.Duration
	t   *pubsub.Topic

	mu sync.Mutex
	p  pulse.Preparer
	hb pulse.Heartbeat
}

func (a *announcer) Start() (err error) {
	a.cq = make(chan struct{})
	a.log = a.log.WithField("ttl", a.ttl)

	a.hb, err = pulse.NewHeartbeat(capnp.SingleSegment(nil))
	if err == nil {
		go a.tick()
	}

	return
}

func (a *announcer) Close() error {
	close(a.cq)
	return nil
}

func (a *announcer) tick() {
	log.Debug("started heartbeat loop")
	defer log.Debug("exited heartbeat loop")

	ticker := jitterbug.New(a.ttl/2, jitterbug.Uniform{
		Min:    a.ttl / 10,
		Source: rand.New(rand.NewSource(rand.Int63())),
	})
	defer ticker.Stop()

	var (
		ctx context.Context = ctxutil.C(a.cq)
		b                   = loggableBackoff{backoff.Backoff{
			Factor: 2,
			Min:    a.ttl / 10,
			Max:    time.Minute * 15,
			Jitter: true,
		}}
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

	a.hb.SetTTL(a.ttl)
	a.p.Prepare(a.hb)

	b, err := a.hb.MarshalBinary()
	if err != nil {
		return err
	}

	// Publish may return nil if the context shuts down.
	err = a.t.Publish(ctx, b, pubsub.WithReadiness(a.ready))
	if err == nil {
		err = ctx.Err()
	}

	return err
}
