//go:generate mockgen -source=multicast.go -destination=../../internal/mock/pkg/boot/multicast.go -package=mock_boot

package boot

import (
	"context"
	"io"
	"math/rand"
	"net"
	"sync/atomic"
	"time"

	"capnproto.org/go/capnp/v3"
	"github.com/jpillora/backoff"
	"github.com/lthibault/jitterbug/v2"
	"github.com/lthibault/log"
	syncutil "github.com/lthibault/util/sync"
	"go.uber.org/fx"

	"github.com/pkg/errors"

	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/libp2p/go-libp2p-core/record"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"

	"github.com/wetware/casm/internal/api/boot"
)

const (
	// TODO:  register protocol at https://github.com/multiformats/multicodec
	P_MCAST         = 0x07
	maxDatagramSize = 8192
)

func init() {
	if err := ma.AddProtocol(ma.Protocol{
		Name:       "multicast",
		Code:       P_MCAST,
		VCode:      ma.CodeToVarint(P_MCAST),
		Size:       ma.LengthPrefixedVarSize,
		Path:       true,
		Transcoder: ma.NewTranscoderFromFunctions(mcastStoB, mcastBtoS, nil),
	}); err != nil {
		panic(err)
	}
}

type (
	Transport interface {
		Dial() (Scatterer, error)
		Listen() (Gatherer, error)
	}

	Scatterer interface {
		Scatter(context.Context, *capnp.Message) error
		Close() error
	}

	Gatherer interface {
		Gather(context.Context) (*capnp.Message, error)
		Close() error
	}

	transportFactory func() (Transport, error)
)

/*
 * Client & Service
 */

type MulticastClient struct {
	client
	runtime fx.Shutdowner
}

// NewMulticast client takes a UDP multiaddr that designates a
// multicast group and returns a multicast discovery client.
func NewMulticastClient(opt ...Option) (c MulticastClient, err error) {
	err = fx.New(fx.NopLogger,
		fx.Supply(opt),
		fx.Populate(&c.client, &c.runtime),
		fx.Provide(
			newConn,
			newConfig,
			newReceiver,
			newClient),
		fx.Invoke(
			hookSender)).
		Start(context.Background())

	return
}

func (m MulticastClient) FindPeers(ctx context.Context, ns string, opt ...discovery.Option) (<-chan peer.AddrInfo, error) {
	return m.client.FindPeers(ctx, normalizeNS(ns), opt...)
}

func (m MulticastClient) Close() error { return m.runtime.Shutdown() }

type Multicast struct {
	client
	server

	runtime fx.Shutdowner
}

// NewMulticast takes a UDP multiaddr that designates a
// multicast group and returns a multicast discovery service.
func NewMulticast(h host.Host, opt ...Option) (m Multicast, err error) {
	err = fx.New(fx.NopLogger,
		fx.Supply(opt),
		fx.Populate(&m.client, &m.server, &m.runtime),
		fx.Provide(
			newConn,
			newConfig,
			newReceiver,
			newSubscriptions,
			newClient,
			newServer,
			newHostComponents(h)),
		fx.Invoke(
			hookSender)).
		Start(context.Background())

	return
}

func (m Multicast) Advertise(ctx context.Context, ns string, opt ...discovery.Option) (time.Duration, error) {
	return m.server.Advertise(ctx, normalizeNS(ns), opt...)
}
func (m Multicast) FindPeers(ctx context.Context, ns string, opt ...discovery.Option) (<-chan peer.AddrInfo, error) {
	return m.client.FindPeers(ctx, normalizeNS(ns), opt...)
}

func (m Multicast) Close() error { return m.runtime.Shutdown() }

type hostComponents struct {
	fx.Out

	ID   peer.ID
	Host host.Host
	Bus  event.Bus
}

func newHostComponents(h host.Host) func() hostComponents {
	return func() hostComponents {
		return hostComponents{
			ID:   h.ID(),
			Host: h,
			Bus:  h.EventBus(),
		}
	}
}

func newConfig(opt []Option) (c Config) {
	c.Apply(opt)
	return
}

func newSubscriptions(bus event.Bus, lx fx.Lifecycle) (sub event.Subscription, err error) {
	if sub, err = bus.Subscribe(new(event.EvtLocalAddressesUpdated)); err == nil {
		hook(lx, closer(sub))
	}

	return
}

type conn struct {
	fx.Out

	Scatterer
	Gatherer
}

func newConn(f transportFactory, lx fx.Lifecycle) (conn, error) {
	var (
		t     Transport
		s     Scatterer
		g     Gatherer
		maybe breaker
	)

	// new transport
	maybe.Do(func() { t, maybe.Err = f() })

	// dial
	maybe.Do(func() { s, maybe.Err = t.Dial() })
	maybe.Do(func() { hook(lx, closer(s)) })

	// listen
	maybe.Do(func() { g, maybe.Err = t.Listen() })
	maybe.Do(func() { hook(lx, closer(g)) })

	return conn{
		Scatterer: s,
		Gatherer:  g,
	}, maybe.Err
}

func hookSender(s Scatterer, outq chan outgoing, lx fx.Lifecycle) (io.Closer, error) {
	// token-bucket rate limiter with burst factor of 8.
	bucket := make(chan struct{}, 8)
	for len(bucket) != cap(bucket) {
		bucket <- struct{}{}
	}

	// jittered ticker replenishes the token bucket
	ticker := jitterbug.New(time.Second, jitterbug.Uniform{
		Source: rand.New(rand.NewSource(time.Now().UnixNano())),
	})

	hook(lx,
		deferred(ticker.Stop),
		goroutine(func() {
			defer close(bucket)

			for range ticker.C {
				select {
				case bucket <- struct{}{}:
				default:
				}

			}
		}))

	// writer loop waits for a token to become available and
	// then sends a packet.
	hook(lx, goroutine(func() {
		for out := range outq {
			select {
			case <-out.C.Done():
			case <-bucket:
				out.Err <- s.Scatter(out.C, out.M)
			}
		}
	}))

	return s, nil
}

// client emits QUERY packets and awaits RESPONSE packet
// from peers.
type client struct {
	cq   chan struct{}
	qs   chan<- outgoing // queries
	add  chan addSink
	rm   chan func()
	recv *receiver
}

func newClient(log log.Logger, qs chan outgoing, r *receiver, lx fx.Lifecycle) client {
	c := client{
		qs:   qs,
		recv: r,
		cq:   make(chan struct{}),
		add:  make(chan addSink),
		rm:   make(chan func()),
	}

	hook(lx,
		deferred(func() { close(qs) }),
		goroutine(func() {
			var m = make(clientTopicManager)
			defer m.Close()
			defer close(c.rm)
			defer close(c.add)
			defer close(c.cq)

			for {
				select {
				case add := <-c.add:
					c.addSink(m, add)

				case free := <-c.rm:
					free()

				case r, ok := <-c.recv.Responses:
					if !ok {
						return
					}

					if err := c.handleResponse(m, r); err != nil {
						log.WithError(err).Debug("malformed response")
						continue
					}
				}
			}
		}))

	return c
}

func (c client) FindPeers(ctx context.Context, ns string, opt ...discovery.Option) (<-chan peer.AddrInfo, error) {
	opts, err := c.options(opt)
	if err != nil {
		return nil, err
	}

	msg, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		return nil, err
	}

	pkt, err := boot.NewRootMulticastPacket(seg)
	if err != nil {
		return nil, err
	}

	if err = pkt.SetQuery(ns); err != nil {
		return nil, err
	}

	out := make(chan peer.AddrInfo, 1)
	sink := &sink{
		NS:   ns,
		ch:   out,
		opts: opts,
	}

	cherr := chErrPool.Get()
	defer chErrPool.Put(cherr)

	add := addSink{
		C:   ctx,
		S:   sink,
		Q:   msg,
		Err: cherr,
	}

	select {
	case c.add <- add:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.cq:
		return nil, errors.New("closing")
	}

	select {
	case err = <-cherr:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.cq:
		return nil, errors.New("closing")
	}

	return out, err
}

func (c client) options(opt []discovery.Option) (opts *discovery.Options, err error) {
	opts = &discovery.Options{}
	if err = opts.Apply(opt...); err == nil {
		if opts.Limit == 0 {
			opts.Limit = -1
		}
	}
	return
}

func (c client) addSink(m clientTopicManager, add addSink) {
	var (
		err = errors.New("closing")
		ctx = add.C
	)

	m.AddSink(add.C, c.cq, add.S)

	cherr := chErrPool.Get()
	defer chErrPool.Put(cherr)

	select {
	case <-c.cq:
	case <-ctx.Done():
	case c.qs <- outgoing{
		C:   add.C,
		M:   add.Q,
		Err: cherr,
	}:
	}

	select {
	case <-c.cq:
	case <-ctx.Done():
	case err = <-cherr:
	}

	if err != nil {
		add.S.Close()
	} else {
		go func() {
			select {
			case <-c.cq:
			case <-ctx.Done():
				select {
				case <-c.cq:
				case c.rm <- add.S.Close:
				}
			}
		}()
	}

	add.Err <- err
}

func (c client) handleResponse(m clientTopicManager, p boot.MulticastPacket) error {
	var rec peer.PeerRecord

	res, err := p.Response()
	if err != nil {
		return err
	}

	ns, err := res.Ns()
	if err != nil {
		return err
	}

	// are we tracking this topic?
	if t, ok := m[ns]; ok {
		b, err := res.SignedEnvelope()
		if err != nil {
			return err
		}

		_, err = record.ConsumeTypedEnvelope(b, &rec)
		if err != nil {
			return err
		}

		t.Consume(c.cq, rec)
	}

	return nil
}

type clientTopicManager map[string]*clientTopic

func (m clientTopicManager) Close() {
	for _, t := range m {
		for _, s := range t.ss {
			s.Close()
		}
	}
}

func (m clientTopicManager) AddSink(ctx context.Context, abort <-chan struct{}, s *sink) {
	t, ok := m[s.NS]
	if !ok {
		t = newClientTopic()
		m[s.NS] = t
	}

	t.ss = append(t.ss, s)
	s.Close = m.newGC(t, s)

	for _, info := range t.seen {
		select {
		case s.ch <- info:
		case <-ctx.Done():
		case <-abort:
		}
	}
}

func (m clientTopicManager) newGC(t *clientTopic, s *sink) func() {
	return func() {
		defer close(s.ch)

		if t.Remove(s) {
			delete(m, s.NS)
		}
	}
}

type clientTopic struct {
	seen map[peer.ID]peer.AddrInfo
	ss   []*sink
}

func newClientTopic() *clientTopic {
	return &clientTopic{
		seen: make(map[peer.ID]peer.AddrInfo),
	}
}

func (t *clientTopic) Consume(abort <-chan struct{}, rec peer.PeerRecord) {
	if _, seen := t.seen[rec.PeerID]; seen {
		return
	}

	info := peer.AddrInfo{ID: rec.PeerID, Addrs: rec.Addrs}
	t.seen[info.ID] = info

	for _, sink := range t.ss {
		sink.Consume(abort, info)
	}
}

func (t *clientTopic) Remove(s *sink) bool {
	for i, x := range t.ss {
		if x == s {
			t.ss[i], t.ss[len(t.ss)-1] = t.ss[len(t.ss)-1], t.ss[i] // mv to last
			t.ss = t.ss[:len(t.ss)-1]                               // pop last
			break
		}
	}

	return len(t.ss) == 0
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
	Q   *capnp.Message // query message
	Err chan<- error
}

// server awaits QUERY packets and replies with RESPONSE packets
type server struct {
	log  log.Logger
	cq   chan struct{}
	ts   map[string]*serverTopic
	add  chan addServerTopic
	e    *record.Envelope
	recv *receiver
}

func newServer(log log.Logger, rs chan outgoing, r *receiver, sub event.Subscription, lx fx.Lifecycle) server {
	m := server{
		log:  log,
		cq:   make(chan struct{}),
		ts:   make(map[string]*serverTopic),
		add:  make(chan addServerTopic, 8),
		recv: r,
	}

	if v, ok := <-sub.Out(); ok { // doesn't block; stateful subscription
		m.e = v.(event.EvtLocalAddressesUpdated).SignedPeerRecord
		hook(lx, goroutine(func() {
			defer close(m.cq)

			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()

			for {
				select {
				case tick := <-ticker.C:
					for _, t := range m.ts {
						if t.Expired(tick) {
							t.Cancel()
						}
					}

				case v := <-sub.Out():
					m.e = v.(event.EvtLocalAddressesUpdated).SignedPeerRecord
					var j syncutil.Join
					for _, t := range m.ts {
						j.Go(m.setRecord(t))
					}

					if err := j.Wait(); err != nil {
						log.WithError(err).Debug("unable to set record")
						continue
					}

				case add := <-m.add:
					add.Err <- m.handleAddTopic(add.NS, add.TTL)

				case pkt, ok := <-m.recv.Queries:
					if !ok {
						return
					}

					ns, _ := pkt.Query()
					if t, ok := m.ts[ns]; ok {
						t.Reply(rs)
					}
				}
			}
		}))
	}

	return m
}

func (s server) Advertise(ctx context.Context, ns string, opt ...discovery.Option) (time.Duration, error) {
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

func (s server) options(opt []discovery.Option) (opts *discovery.Options, err error) {
	opts = &discovery.Options{}
	if err = opts.Apply(opt...); err == nil {
		if opts.Ttl == 0 {
			opts.Ttl = peerstore.PermanentAddrTTL
		}
	}
	return
}

func (s server) handleAddTopic(ns string, ttl time.Duration) (err error) {
	t, ok := s.ts[ns]
	if !ok {
		if t, err = s.newServerTopic(ns, func() {
			delete(s.ts, ns)
		}); err == nil {
			s.ts[ns] = t
		}
	}

	if err == nil {
		t.SetTTL(ttl)
		err = t.SetRecord(s.e)
	}

	return
}

func (s server) setRecord(t *serverTopic) func() error {
	return func() error { return t.SetRecord(s.e) }
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

func (s server) newServerTopic(ns string, stop func()) (*serverTopic, error) {
	msg, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		return nil, err
	}

	pkt, err := boot.NewRootMulticastPacket(seg)
	if err != nil {
		return nil, err
	}

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
				case rs <- outgoing{C: ctx, M: msg, Err: cherr}:
				case <-ctx.Done():
				}

				select {
				case err := <-cherr:
					if err != nil {
						s.log.WithError(err).Debug("failed to send response")
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

type receiver struct {
	Queries, Responses chan boot.MulticastPacket
}

func newReceiver(log log.Logger, g Gatherer, opt []Option, lx fx.Lifecycle) *receiver {
	var (
		cancel context.CancelFunc

		r = &receiver{
			Queries:   make(chan boot.MulticastPacket, 8),
			Responses: make(chan boot.MulticastPacket, 8),
		}

		b = backoff.Backoff{
			Factor: 2,
			Jitter: true,
			Min:    time.Second,
			Max:    time.Minute * 5,
		}
	)

	hook(lx,
		deferred(func() {
			cancel()
			close(r.Queries)
			close(r.Responses)
		}),
		goWithContext(func(ctx context.Context) {
			ctx, cancel = context.WithCancel(ctx)

			for {
				switch msg, err := g.Gather(ctx); err {
				case context.Canceled, context.DeadlineExceeded:
					return

				case nil:
					b.Reset()

					if err = r.consume(ctx, msg); err != nil {
						log.WithError(err).Warn("malformed packet")
					}

				default:
					if ne, ok := err.(net.Error); ok && !ne.Temporary() {
						log.WithError(err).Fatal("network error")
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
		}))

	return r
}

func (r receiver) consume(ctx context.Context, msg *capnp.Message) error {
	var (
		p, err   = boot.ReadRootMulticastPacket(msg)
		consumer chan<- boot.MulticastPacket
	)

	if err == nil {
		switch p.Which() {
		case boot.MulticastPacket_Which_query:
			consumer = r.Queries
		case boot.MulticastPacket_Which_response:
			consumer = r.Responses
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
	M   *capnp.Message
	Err chan<- error
}

/*
 * Transports
 */

type MulticastUDP struct {
	local atomic.Value // net.Addr
	Group *net.UDPAddr
}

// NewMulticastUDP transport.  The multiaddr 'm' MUST contain an
// *unencapsulated* UDP address.
//
// For the avoidance of doubt:  NewMulticastUDP will return a non-nil
// error if 'm' is of type P_MCAST.
func NewMulticastUDP(m ma.Multiaddr) (*MulticastUDP, error) {
	network, addr, err := manet.DialArgs(m)
	if err != nil {
		return nil, err
	}

	udpAddr, err := net.ResolveUDPAddr(network, addr)
	if err != nil {
		return nil, err
	}

	return &MulticastUDP{Group: udpAddr}, nil
}

func (u *MulticastUDP) Dial() (Scatterer, error) {
	conn, err := net.DialUDP(u.Group.Network(), nil, u.Group)
	if err != nil {
		return nil, err
	}
	u.local.Store(conn.LocalAddr())

	if err = conn.SetWriteBuffer(maxDatagramSize); err != nil {
		return nil, err
	}

	return scatterUDP{conn}, nil
}

func (u *MulticastUDP) Listen() (Gatherer, error) {
	conn, err := net.ListenMulticastUDP(u.Group.Network(), nil, u.Group)
	if err != nil {
		return nil, err
	}

	if err = conn.SetReadBuffer(maxDatagramSize); err != nil {
		return nil, err
	}

	return gatherUDP{
		UDPConn: conn,
		local:   &u.local,
	}, nil
}

type scatterUDP struct{ *net.UDPConn }

func (s scatterUDP) Scatter(ctx context.Context, msg *capnp.Message) error {
	b, err := msg.MarshalPacked()
	if err != nil {
		return err
	}

	dl, _ := ctx.Deadline()
	if err = s.SetWriteDeadline(dl); err == nil {
		_, err = s.Write(b)
	}

	return err
}

type gatherUDP struct {
	*net.UDPConn
	local *atomic.Value
}

func (g gatherUDP) Gather(ctx context.Context) (*capnp.Message, error) {
	b := make([]byte, maxDatagramSize) // TODO:  pool

	for {
		dl, _ := ctx.Deadline()
		if err := g.SetReadDeadline(dl); err != nil {
			return nil, err
		}

		n, ret, err := g.ReadFrom(b)
		if err != nil {
			return nil, err
		}

		// UDP packet did not come from us?
		if v := g.local.Load(); v == nil || ret.String() != v.(net.Addr).String() {
			return capnp.UnmarshalPacked(b[:n]) // TODO:  pool message
		}
	}
}

/*
 * P_MCAST
 */

func mcastStoB(s string) ([]byte, error) {
	m, err := ma.NewMultiaddr(s)
	if err != nil {
		return nil, err
	}

	return m.Bytes(), nil
}

func mcastBtoS(b []byte) (string, error) {
	m, err := ma.NewMultiaddrBytes(b)
	if err != nil {
		return "", err
	}
	return m.String(), nil
}

type hookFunc func(*fx.Hook)

func hook(lx fx.Lifecycle, hfs ...hookFunc) {
	var h fx.Hook
	for _, apply := range hfs {
		apply(&h)
	}
	lx.Append(h)
}

func setup(f func(context.Context) error) hookFunc {
	return func(h *fx.Hook) { h.OnStart = f }
}

func goroutine(f func()) hookFunc {
	return goWithContext(func(context.Context) { go f() })
}

func goWithContext(f func(context.Context)) hookFunc {
	return setup(func(c context.Context) error {
		go f(c)
		return nil
	})
}

func deferred(f func()) hookFunc {
	return func(h *fx.Hook) {
		h.OnStop = func(context.Context) error {
			f()
			return nil
		}
	}
}

func closer(c io.Closer) hookFunc {
	return func(h *fx.Hook) {
		h.OnStop = func(context.Context) error {
			return c.Close()
		}
	}
}
