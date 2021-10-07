//go:generate mockgen -source=multicast.go -destination=../../../internal/mock/pkg/boot/multicast/multicast.go -package=mock_boot

package multicast

import (
	"context"
	"net"
	"time"

	"capnproto.org/go/capnp/v3"
	"github.com/jpillora/backoff"
	"github.com/lthibault/log"
	"go.uber.org/fx"

	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/wetware/casm/internal/api/boot"
)

type (
	// // Transport is a factory type that constructs a Scatterer/Gatherer
	// // pair.  Calls to NewScatter and NewGather MUST be idempotent.
	// Transport interface {
	// 	NewScatter(context.Context) (Scatterer, error)
	// 	NewGather(context.Context) (Gatherer, error)
	// }

	Scatterer interface {
		Scatter(context.Context, *capnp.Message) error
	}

	Gatherer interface {
		Gather(context.Context) (*capnp.Message, error)
	}
)

type Config struct {
	fx.In `ignore-unexported:"true"`

	Log       log.Logger `optional:"true"`
	Scatterer Scatterer
	Gatherer  Gatherer

	pr *packetRouter
	qs chan outgoing // outgoing queries
}

func (cfg *Config) logger() log.Logger {
	if cfg.Log == nil {
		cfg.Log = log.New(log.WithLevel(log.FatalLevel))
	}

	return cfg.Log
}

func (cfg *Config) queryChan() chan outgoing {
	if cfg.qs == nil {
		cfg.qs = make(chan outgoing, 8)
	}

	return cfg.qs
}

func (cfg *Config) router(ctx context.Context) *packetRouter {
	if cfg.pr == nil {
		cfg.pr = newPacketRouter(ctx, cfg.logger(), cfg.Gatherer)
	}

	return cfg.pr
}

// TODO(performance):  rate-limit sending via token bucket.
func sendLoop(cfg Config) {
	for out := range cfg.queryChan() {
		select {
		case <-out.C.Done():
		case out.Err <- cfg.Scatterer.Scatter(out.C, out.P.Message()):
		}
	}
}

type sink struct {
	NS string

	ch    chan<- peer.AddrInfo
	Close func()

	opts *discovery.Options
}

func (s sink) Consume(abort <-chan struct{}, info peer.AddrInfo) {
	select {
	case s.ch <- info:
		if s.opts.Limit > 0 {
			if s.opts.Limit--; s.opts.Limit == 0 {
				s.Close() // Consume is always called from a select statement
			}
		}

	case <-abort:
	}
}

type addSink struct {
	C   context.Context
	S   *sink
	P   *boot.MulticastPacket
	Err chan<- error
}

type packetRouter struct{ qs, rs chan boot.MulticastPacket }

func (r packetRouter) Queries() <-chan boot.MulticastPacket   { return r.qs }
func (r packetRouter) Responses() <-chan boot.MulticastPacket { return r.rs }

func newPacketRouter(ctx context.Context, log log.Logger, g Gatherer) *packetRouter {
	r := &packetRouter{
		qs: make(chan boot.MulticastPacket, 8),
		rs: make(chan boot.MulticastPacket, 8),
	}

	b := backoff.Backoff{
		Factor: 2,
		Jitter: true,
		Min:    time.Second,
		Max:    time.Minute * 5,
	}

	go func() {
		defer close(r.qs)
		defer close(r.rs)

		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		for {
			switch msg, err := g.Gather(ctx); err {
			case context.Canceled, context.DeadlineExceeded:
				return

			case nil:
				b.Reset()

				if err = r.consume(ctx, msg); err != nil {
					log.WithError(err).
						Warn("malformed packet")
				}

			default:
				if ne, ok := err.(net.Error); ok && !ne.Temporary() {
					log.WithError(err).
						Fatal("network error")
				}

				log.WithError(err).
					WithField("backoff", b.ForAttempt(b.Attempt())).
					Debug("entering backoff state")

				select {
				case <-time.After(b.Duration()):
				case <-ctx.Done():
				}
			}
		}
	}()

	return r
}

func (r packetRouter) consume(ctx context.Context, msg *capnp.Message) error {
	var (
		p, err   = boot.ReadRootMulticastPacket(msg)
		consumer chan<- boot.MulticastPacket
	)

	if err == nil {
		switch p.Which() {
		case boot.MulticastPacket_Which_query:
			consumer = r.qs
		case boot.MulticastPacket_Which_response:
			consumer = r.rs
		}

		select {
		case consumer <- p:
		case <-ctx.Done():
			err = ctx.Err()
		}
	}

	return err
}

type outgoing struct {
	C   context.Context
	P   *boot.MulticastPacket
	Err chan<- error
}
