//go:generate mockgen -source=cluster.go -destination=../../internal/mock/pkg/cluster/cluster.go -package=mock_cluster

// Package cluster exports an asynchronously updated model of the swarm.
package cluster

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/jpillora/backoff"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/lthibault/jitterbug/v2"
	"github.com/lthibault/log"
	ctxutil "github.com/lthibault/util/ctx"
	"github.com/wetware/casm/pkg/cluster/pulse"
	"github.com/wetware/casm/pkg/cluster/routing"
)

type Topic interface {
	String() string
	Publish(context.Context, []byte, ...pubsub.PubOpt) error
	Relay() (pubsub.RelayCancelFunc, error)
}

// RoutingTable tracks the liveness of cluster peers and provides a
// simple API for querying routing information.
type RoutingTable interface {
	Advance(time.Time)
	Upsert(routing.Record) (created bool)
	Snapshot() routing.Snapshot
}

// Router is a peer participating in the cluster membership protocol.
// It maintains a global view of the cluster with PA/EL guarantees,
// and periodically announces its presence to others.
type Router struct {
	Topic Topic

	Log          log.Logger
	TTL          time.Duration
	Meta         pulse.Preparer
	RoutingTable RoutingTable

	once     sync.Once
	err      error
	clock    *time.Ticker
	cancel   pubsub.RelayCancelFunc
	hb       pulse.Heartbeat
	done     chan struct{}
	announce chan announcement
}

func (r *Router) Stop() {
	r.once.Do(func() {})

	if r.clock != nil {
		r.clock.Stop()
		r.cancel()
	}
}

func (r *Router) String() string {
	return r.Topic.String()
}

func (r *Router) ID() routing.ID {
	return r.hb.Instance()
}

func (r *Router) Loggable() map[string]any {
	return r.hb.Loggable()
}

func (r *Router) View() View {
	return Server{RoutingTable: r.RoutingTable}.View()
}

func (r *Router) Bootstrap(ctx context.Context, opt ...pubsub.PubOpt) error {
	if err := r.initialize(); err != nil {
		return err
	}

	request := bootstrap(opt)
	select {
	case r.announce <- request:
		return request.Wait(ctx)

	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *Router) initialize() error {
	r.once.Do(func() {
		if r.Log == nil {
			r.Log = log.New()
		}

		if r.RoutingTable == nil {
			r.RoutingTable = routing.New(time.Now())
		}

		if r.Meta == nil {
			r.Meta = nopPreparer{}
		}

		if r.TTL <= 0 {
			r.TTL = pulse.DefaultTTL
		}

		// Start relaying messages.  Note that this will not populate
		// the routing table unless pulse.Validator was previously set.
		if r.cancel, r.err = r.Topic.Relay(); r.err == nil {
			r.hb = pulse.NewHeartbeat()
			r.Log = r.Log.With(r.hb)
			r.clock = time.NewTicker(time.Millisecond * 10)
			r.done = make(chan struct{})
			r.announce = make(chan announcement, 1)

			go r.advance()
			go r.heartbeat()
		}
	})

	return r.err
}

func (r *Router) advance() {
	defer close(r.announce)
	defer close(r.done)

	var announce = announcement{
		cherr: make(chan error, 1),
	}

	jitter := jitterbug.Uniform{
		Min:    r.TTL / 10,
		Source: rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	for t := range r.clock.C {
		if r.RoutingTable.Advance(t); announce.Ready(t) {
			next := t.Add(jitter.Jitter(r.TTL / 2))
			select {
			case r.announce <- announce.WithDeadline(next):
			default:
			}
		}
	}
}

func (r *Router) heartbeat() {
	backoff := &loggableBackoff{backoff.Backoff{
		Factor: 2,
		Min:    r.TTL / 2,
		Max:    time.Minute * 15,
		Jitter: true,
	}}

	ctx := ctxutil.C(r.done)

	for a := range r.announce {
		err := r.emit(ctx, a)
		if err == nil {
			backoff.Reset()
			continue
		}

		// shutting down?
		if err == context.Canceled {
			return
		}

		r.Log.
			With(backoff).
			WithError(err).
			Warn("failed to announce")

		// don't retry if the context is expired
		if err == context.DeadlineExceeded {
			continue
		}

		// back off...
		select {
		case <-time.After(backoff.Duration()):
			r.Log.Debug("resuming")
		case <-r.done:
			return
		}

		// ... and retry
		select {
		case r.announce <- a:
			r.Log.Debug("retrying announce")
		default:
		}
	}
}

func (r *Router) emit(ctx context.Context, a announcement) (err error) {
	if err = r.Meta.Prepare(r.hb); err == nil {
		err = a.Send(ctx, r.Topic, r.hb)
	}

	return
}

type announcement struct {
	opt   []pubsub.PubOpt
	dl    time.Time
	cherr chan error
}

func bootstrap(opt []pubsub.PubOpt) announcement {
	return announcement{
		opt:   opt,
		cherr: make(chan error, 1),
	}
}

func (a announcement) Wait(ctx context.Context) error {
	select {
	case err := <-a.cherr:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (a announcement) Send(ctx context.Context, t Topic, h pulse.Heartbeat) error {
	ctx, cancel := a.withDeadline(ctx)
	defer cancel()

	msg, err := h.Message().MarshalPacked()
	if err == nil {
		err = t.Publish(ctx, msg, a.opt...)
	}

	if a.cherr != nil {
		a.cherr <- err
	}

	return err
}

func (a announcement) WithDeadline(dl time.Time) announcement {
	a.dl = dl.Truncate(time.Millisecond)
	return a
}

func (a announcement) Ready(t time.Time) bool { return !t.Before(a.dl) }

func (a announcement) withDeadline(ctx context.Context) (context.Context, context.CancelFunc) {
	if a.dl.IsZero() {
		return ctx, func() {}
	}

	return context.WithDeadline(ctx, a.dl)
}

type loggableBackoff struct{ backoff.Backoff }

func (b *loggableBackoff) Loggable() map[string]interface{} {
	return map[string]interface{}{
		"attempt": int(b.Attempt()),
		"dur":     b.ForAttempt(b.Attempt()),
		"max_dur": b.Max,
	}
}

type nopPreparer struct{}

func (nopPreparer) Prepare(pulse.Heartbeat) error { return nil }
