//go:generate mockgen -source=cluster.go -destination=../../internal/mock/pkg/cluster/cluster.go -package=mock_cluster

// Package cluster exports an asynchronously updated model of the swarm.
package cluster

import (
	"context"
	"errors"
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
	id       uint32 // instance ID
	cancel   pubsub.RelayCancelFunc
	done     chan struct{}
	announce chan []pubsub.PubOpt
}

func (r *Router) Stop() {
	r.once.Do(func() {
		r.done = make(chan struct{})
		r.cancel = func() {}
	})

	close(r.done)
	r.cancel()
}

func (r *Router) String() string {
	return r.Topic.String()
}

func (r *Router) ID() routing.ID {
	return routing.ID(r.id)
}

func (r *Router) Loggable() map[string]any {
	return map[string]any{
		"server": r.ID(),
		"ttl":    r.TTL,
	}
}

func (r *Router) View() View {
	return Server{RoutingTable: r.RoutingTable}.View()
}

func (r *Router) Bootstrap(ctx context.Context, opt ...pubsub.PubOpt) error {
	if err := r.initialize(); err != nil {
		return err
	}

	select {
	case r.announce <- opt:
		return nil

	case <-r.done:
		return errors.New("closing")

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
			r.id = rand.Uint32()
			r.Log = r.Log.With(r)
			r.done = make(chan struct{})
			r.announce = make(chan []pubsub.PubOpt)

			go r.advance()
			go r.heartbeat()
		}
	})

	return r.err
}

func (r *Router) advance() {
	defer close(r.announce)

	clock := time.NewTicker(time.Millisecond * 10)
	defer clock.Stop()

	ticker := jitterbug.New(r.TTL/2, jitterbug.Uniform{
		Min:    r.TTL/2 - 1,
		Source: rand.New(rand.NewSource(time.Now().UnixNano())),
	})
	defer ticker.Stop()

	for {
		select {
		case t := <-clock.C:
			r.RoutingTable.Advance(t)

		case <-ticker.C:
			select {
			case r.announce <- nil:
			default:
			}

		case <-r.done:
			return
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

	hb := pulse.NewHeartbeat()
	hb.SetTTL(r.TTL)
	hb.SetServer(r.ID())

	ctx := ctxutil.C(r.done)

	for a := range r.announce {
		err := r.emit(ctx, hb, a)
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

		// back off...
		select {
		case <-time.After(backoff.Duration()):
			r.Log.Debug("resuming")
		case <-r.done:
			return
		}
	}
}

func (r *Router) emit(ctx context.Context, hb pulse.Heartbeat, opt []pubsub.PubOpt) error {
	if err := r.Meta.Prepare(hb); err != nil {
		return err
	}

	msg, err := hb.Message().MarshalPacked()
	if err != nil {
		return err
	}

	return r.Topic.Publish(ctx, msg, opt...)
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
