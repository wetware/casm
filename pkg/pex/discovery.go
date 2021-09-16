package pex

import (
	"context"
	"errors"
	"time"

	"github.com/jpillora/backoff"
	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/peer"
	ps "github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/lthibault/log"
	"go.uber.org/fx"
)

var b = backoff.Backoff{
	Factor: 2,
	Jitter: true,
	Min:    time.Second,
	Max:    time.Minute * 30,
}

type discover struct {
	log log.Logger
	d   discovery.Discovery
	opt []discovery.Option

	ctx     context.Context
	ts      chan trackRequest
	expired chan string
}

type discoverParam struct {
	fx.In

	Log  log.Logger
	Disc discovery.Discovery
	Opt  []discovery.Option
}

func newDiscover(p discoverParam, lx fx.Lifecycle) *discover {
	disc := &discover{
		log: p.Log,
		d:   p.Disc,
		opt: p.Opt,

		ts:      make(chan trackRequest),
		expired: make(chan string),
	}

	lx.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			disc.ctx = ctx
			go disc.loop()
			return nil
		},
	})

	return disc
}

func (d *discover) Discover(ctx context.Context, ns string) (<-chan peer.AddrInfo, error) {
	if d.ctx.Err() != nil {
		return nil, errors.New("closed")
	}

	ctx, cancel := context.WithCancel(ctx)
	go func() {
		defer cancel()

		select {
		case <-ctx.Done():
		case <-d.ctx.Done():
		}
	}()

	return d.d.FindPeers(ctx, ns, d.opt...)
}

func (d *discover) Track(ctx context.Context, ns string, ttl time.Duration) error {
	if d == nil {
		return nil
	}

	sync := chSyncPool.Get()
	defer chSyncPool.Put(sync)

	req := trackRequest{
		ns:   ns,
		ttl:  ttl,
		sync: sync,
	}

	select {
	case d.ts <- req:
	case <-ctx.Done():
	case <-d.ctx.Done():
	}

	select {
	case <-sync:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-d.ctx.Done():
		return errors.New("closing")
	}
}

func (d *discover) loop() {
	topics := make(map[string]topic)

	for {
		select {
		case ns := <-d.expired:
			delete(topics, ns)

		case req := <-d.ts:
			t, ok := topics[req.ns]
			if !ok {
				t = d.newTopic(req.ns)
				topics[req.ns] = t
			}

			t.SetTTL(d.ctx, req.ttl)
			req.Finish()

		case <-d.ctx.Done():
			return
		}
	}
}

func (d *discover) newTopic(ns string) topic {
	ctx, cancel := context.WithCancel(d.ctx)
	reset := make(topic)

	// HACK:  we use an error chan to synchronize.  This helps with
	//        testing, as we can be sure that all goroutines have
	//        started when newTopic() returns.
	sync := chSyncPool.Get()
	defer chSyncPool.Put(sync)

	// Monitor the TTL for the entire namespace.  If the PeerExchange
	// does not re-advertise periodically, we stop the bootstrapper's
	// ad loop.
	go func() {
		defer cancel()

		timer := time.NewTimer(ps.PermanentAddrTTL)
		defer timer.Stop()

		sync.Signal() // signal goroutine start

		for {
			select {
			case <-timer.C:
			case <-ctx.Done():
			case ttl := <-reset:
				// We extend the TTL by 5% because in practice, the
				// next call to Track() will occur slightly *after*
				// the TTL, causing the topic to be dropped moments
				// before being re-added.  In these circumstances a
				// concurrent call to Discover() will fail with the
				// "not tracking namespace" error.
				timer.Reset(ttl + (ttl / 20))
				continue
			}

			select {
			case <-ctx.Done():
			case d.expired <- ns:
			}

			break
		}
	}()

	<-sync

	// Start the bootstrapper's ad loop.  We periodically re-advertise
	// our presence on the bootstrap service. The interval between ads
	// is determined by the TTL return value.
	go func() {
		log := d.log.WithField("ns", ns)

		timer := time.NewTimer(ps.PermanentAddrTTL)
		defer timer.Stop()

		sync.Signal() // signal goroutine start

		for i := 0; ctx.Err() == nil; i++ {
			next, err := d.d.Advertise(ctx, ns, d.opt...)
			switch {
			case err == nil:
				i = 0 // reset backoff

			case errors.Is(err, context.Canceled):
				return

			default:
				if next == 0 {
					next = b.ForAttempt(float64(i))
				}
				log.WithError(err).
					WithField("backoff", next).
					Warn("bootstrap failed")
			}

			timer.Reset(next)

			select {
			case <-timer.C:
			case <-ctx.Done():
			}
		}
	}()

	<-sync
	return reset
}

type topic chan time.Duration

func (t topic) SetTTL(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case t <- d:
	}
}

type trackRequest struct {
	ns   string
	ttl  time.Duration
	sync interface{ Signal() }
}

func (r trackRequest) Finish() { r.sync.Signal() }
