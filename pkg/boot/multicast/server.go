package multicast

import (
	"context"
	"time"

	"github.com/pkg/errors"

	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/libp2p/go-libp2p-core/record"

	"github.com/lthibault/log"
	syncutil "github.com/lthibault/util/sync"

	"github.com/wetware/casm/internal/api/boot"
)

type QueryStream interface {
	Queries() <-chan boot.MulticastPacket
}

// Server awaits QUERY packets and replies with RESPONSE packets
type Server struct {
	cq   <-chan struct{}
	ts   map[string]*serverTopic
	add  chan addServerTopic
	recv QueryStream
}

// NewServer takes a UDP multiaddr that designates a
// multicast group and returns a multicast discovery service.
func NewServer(ctx context.Context, h host.Host, cfg Config) (Server, error) {
	return newServerFactory(ctx).
		Bind(serverConfig(cfg)).
		Bind(serverEventLoop(cfg, h.EventBus())).
		Bind(serverSendLoop(cfg)).
		Return()
}

func (s Server) Advertise(ctx context.Context, ns string, opt ...discovery.Option) (time.Duration, error) {
	opts, err := s.options(opt)
	if err != nil {
		return 0, err
	}

	cherr := chErrPool.Get()
	defer chErrPool.Put(cherr)

	select {
	case s.add <- addServerTopic{NS: ns, TTL: opts.Ttl, Err: cherr}:
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-s.cq:
		return 0, errors.New("closing")
	}

	select {
	case err = <-cherr:
		return opts.Ttl, err
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-s.cq:
		return 0, errors.New("closing")
	}
}

func (s Server) options(opt []discovery.Option) (opts *discovery.Options, err error) {
	opts = &discovery.Options{}
	if err = opts.Apply(opt...); err == nil {
		if opts.Ttl == 0 {
			opts.Ttl = peerstore.PermanentAddrTTL
		}
	}
	return
}

func (s Server) handleAddTopic(log log.Logger, e *record.Envelope, add addServerTopic) (err error) {
	t, ok := s.ts[add.NS]
	if !ok {
		if t, err = s.newServerTopic(log, add.NS, func() {
			delete(s.ts, add.NS)
		}); err == nil {
			s.ts[add.NS] = t
		}
	}

	if err == nil {
		t.SetTTL(add.TTL)
		err = t.SetRecord(e)
	}

	return
}

func (s Server) setRecord(t *serverTopic, e *record.Envelope) func() error {
	return func() error { return t.SetRecord(e) }
}

type serverFactory struct {
	ctx    context.Context
	cancel context.CancelFunc

	err error
	s   Server
}

func newServerFactory(ctx context.Context) (f serverFactory) {
	f.ctx, f.cancel = context.WithCancel(ctx)
	return f
}

func justServer(s Server) serverFactory     { return serverFactory{s: s} }
func serverFailure(err error) serverFactory { return serverFactory{err: err} }

func (sf serverFactory) Bind(f func(context.Context, Server) serverFactory) serverFactory {
	if sf.err != nil {
		sf.cancel()
		return sf
	}

	return f(sf.ctx, sf.s)
}

func (sf serverFactory) Return() (Server, error) { return sf.s, sf.err }

func serverConfig(cfg Config) func(context.Context, Server) serverFactory {
	return func(ctx context.Context, s Server) serverFactory {
		s.cq = ctx.Done()
		s.ts = make(map[string]*serverTopic)
		s.add = make(chan addServerTopic, 8)
		s.recv = cfg.router(ctx)
		return justServer(s)
	}
}

func serverEventLoop(cfg Config, bus event.Bus) func(context.Context, Server) serverFactory {
	return func(ctx context.Context, s Server) serverFactory {
		sub, err := bus.Subscribe(new(event.EvtLocalAddressesUpdated))
		if err != nil {
			return serverFailure(err)
		}

		if v, ok := <-sub.Out(); ok { // doesn't block; stateful subscription
			e := v.(event.EvtLocalAddressesUpdated).SignedPeerRecord
			return justServer(s).Bind(eventloop(cfg, sub, e))
		}

		return serverFailure(errors.New("host closing"))
	}
}

func eventloop(cfg Config, sub event.Subscription, e *record.Envelope) func(context.Context, Server) serverFactory {
	return func(ctx context.Context, s Server) serverFactory {
		go func() {
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()

			for {
				select {
				case tick := <-ticker.C:
					for _, t := range s.ts {
						if t.Expired(tick) {
							t.Cancel()
						}
					}

				case v := <-sub.Out():
					e = v.(event.EvtLocalAddressesUpdated).SignedPeerRecord
					var j syncutil.Join
					for _, t := range s.ts {
						j.Go(s.setRecord(t, e))
					}

					if err := j.Wait(); err != nil {
						cfg.logger().WithError(err).Debug("unable to set record")
						continue
					}

				case add := <-s.add:
					add.Err <- s.handleAddTopic(cfg.logger(), e, add)

				case pkt, ok := <-s.recv.Queries():
					if !ok {
						return
					}

					ns, _ := pkt.Query()
					if t, ok := s.ts[ns]; ok {
						t.Reply(cfg.queryChan())
					}
				}
			}
		}()

		return justServer(s)
	}
}

func serverSendLoop(cfg Config) func(context.Context, Server) serverFactory {
	return func(ctx context.Context, s Server) serverFactory {
		go sendLoop(cfg)
		return justServer(s)
	}
}

type addServerTopic struct {
	NS  string
	TTL time.Duration
	Err chan<- error
}

type serverTopic struct {
	cq      <-chan struct{}
	expire  time.Time
	dedup   chan<- chan<- outgoing
	updates chan<- update
	Cancel  func()
}

func (s Server) newServerTopic(log log.Logger, ns string, stop func()) (*serverTopic, error) {
	pkt, err := packetPool.GetResponsePacket()
	if err != nil {
		return nil, err
	}
	defer packetPool.Put(pkt)

	res, err := pkt.NewResponse()
	if err != nil {
		return nil, err
	}

	if err = res.SetNs(ns); err != nil {
		return nil, err
	}

	var (
		dedup       = make(chan chan<- outgoing)
		updates     = make(chan update)
		ctx, cancel = context.WithCancel(context.Background())
	)

	go func() {
		defer close(updates)
		defer close(dedup)

		cherr := chErrPool.Get()
		defer chErrPool.Put(cherr)

		for {
			select {
			case rs := <-dedup:
				select {
				case rs <- outgoing{C: ctx, P: pkt, Err: cherr}:
				case <-ctx.Done():
				}

				select {
				case err := <-cherr:
					if err != nil {
						log.WithError(err).Debug("failed to send response")
					}

				case <-ctx.Done():

				}

			case u := <-updates:
				b, err := u.E.Marshal()
				if err == nil {
					err = res.SetSignedEnvelope(b)
				}
				u.Err <- err // never blocks

			case <-ctx.Done():
				return
			}
		}
	}()

	return &serverTopic{
		cq:      s.cq,
		dedup:   dedup,
		updates: updates,
		Cancel: func() {
			stop()
			cancel()
		},
	}, nil
}

func (s *serverTopic) Expired(t time.Time) bool { return t.After(s.expire) }
func (s *serverTopic) SetTTL(d time.Duration)   { s.expire = time.Now().Add(d) }

func (s *serverTopic) Reply(rs chan<- outgoing) {
	select {
	case s.dedup <- rs: // duplicate suppression
	default:
	}
}

func (s *serverTopic) SetRecord(e *record.Envelope) error {
	cherr := chErrPool.Get()
	defer chErrPool.Put(cherr)

	err := errors.New("closing")

	select {
	case s.updates <- update{E: e, Err: cherr}:
	case <-s.cq:
		return err
	}

	select {
	case err = <-cherr:
	case <-s.cq:
	}

	return err
}

type update struct {
	E   *record.Envelope
	Err chan<- error
}
