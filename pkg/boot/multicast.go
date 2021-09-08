package boot

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net"
	"reflect"
	"sync/atomic"
	"time"

	"capnproto.org/go/capnp/v3"
	"github.com/hashicorp/go-multierror"
	"github.com/jpillora/backoff"
	"github.com/lthibault/jitterbug/v2"
	"github.com/lthibault/log"
	syncutil "github.com/lthibault/util/sync"

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

/*
 * Client & Service
 */

type MulticastClient struct {
	conn   io.Closer
	client multicastClient
}

// NewMulticast client takes a UDP multiaddr that designates a
// multicast group and returns a multicast discovery client.
func NewMulticastClient(m ma.Multiaddr, opt ...Option) (*MulticastClient, error) {
	conn, err := newMulticastConn(m, opt)
	if err != nil {
		return nil, err
	}

	return &MulticastClient{
		conn:   conn,
		client: conn.NewClient(),
	}, nil
}

func (m MulticastClient) FindPeers(ctx context.Context, ns string, opt ...discovery.Option) (<-chan peer.AddrInfo, error) {
	return m.client.FindPeers(ctx, normalizeNS(ns), opt)
}

func (m MulticastClient) Close() error { return m.conn.Close() }

type Multicast struct {
	conn   io.Closer
	client multicastClient
	server multicastServer
}

// NewMulticast takes a UDP multiaddr that designates a
// multicast group and returns a multicast discovery service.
func NewMulticast(h host.Host, m ma.Multiaddr, opt ...Option) (*Multicast, error) {
	conn, err := newMulticastConn(m, opt)
	if err != nil {
		return nil, err
	}

	server, err := conn.NewServer(h.EventBus())
	return &Multicast{
		conn:   conn,
		client: conn.NewClient(),
		server: server,
	}, err
}

func (m Multicast) Advertise(ctx context.Context, ns string, opt ...discovery.Option) (time.Duration, error) {
	return m.server.Advertise(ctx, normalizeNS(ns), opt)
}
func (m Multicast) FindPeers(ctx context.Context, ns string, opt ...discovery.Option) (<-chan peer.AddrInfo, error) {
	return m.client.FindPeers(ctx, normalizeNS(ns), opt)
}

func (m Multicast) Close() error { return m.conn.Close() }

/*
 * Unexported client/server implementations & utilities.
 */

type multicastConn struct {
	log log.Logger
	out chan outgoing
	*receiver
	sender io.Closer
}

func newMulticastConn(m ma.Multiaddr, opt []Option) (*multicastConn, error) {
	t, err := NewMulticastTransport(m)
	if err != nil {
		return nil, err
	}

	out := make(chan outgoing, 8)

	recv, err := newRecver(t, opt)
	if err != nil {
		return nil, err
	}

	send, err := newSender(t, out)
	if err != nil {
		return nil, err
	}

	return &multicastConn{
		out:      out,
		log:      recv.log,
		receiver: recv,
		sender:   send,
	}, err
}

func (conn multicastConn) NewClient() multicastClient {
	var (
		c = newMulticastClient(conn)
		m = make(clientTopicManager)
	)

	go func() {
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
					conn.log.WithError(err).Debug("malformed response")
					continue
				}
			}
		}
	}()

	return c
}

func (conn multicastConn) NewServer(bus event.Bus) (multicastServer, error) {
	m := multicastServer{
		log:  conn.log,
		cq:   make(chan struct{}),
		ts:   make(map[string]*serverTopic),
		add:  make(chan addServerTopic, 8),
		recv: conn.receiver,
	}

	sub, err := bus.Subscribe(new(event.EvtLocalAddressesUpdated))
	if err != nil {
		return m, err
	}

	if v, ok := <-sub.Out(); ok { // doesn't block; stateful subscription
		m.e = v.(event.EvtLocalAddressesUpdated).SignedPeerRecord

		go func() {
			defer sub.Close()
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
						conn.log.WithError(err).Debug("unable to set record")
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
						t.Reply(conn.out)
					}
				}
			}
		}()
	}

	return m, err
}

func (conn multicastConn) Close() error {
	return multierror.Append(
		conn.receiver.Close(),
		conn.sender.Close())
}

// scatterer and gatherer are interfaces because we may need to drop
// down to the lower-level x/ip4 and x/ip6 libraries to fine-tune
// multicast settings.
type (
	Scatterer interface {
		Scatter(context.Context, *capnp.Message) error
		io.Closer
	}

	Gatherer interface {
		Gather(context.Context) (*capnp.Message, error)
		io.Closer
	}

	multicastDialer interface {
		DialMulticast() (Scatterer, error)
	}

	multicastListener interface {
		ListenMulticast() (Gatherer, error)
	}
)

type MulticastTransport interface {
	DialMulticast() (Scatterer, error)
	ListenMulticast() (Gatherer, error)
}

// NewMulticastTransport resolves 'm' into a generic transport for
// multicast discovery.  Returns an error if 'm' is not of type P_MCAST.
func NewMulticastTransport(m ma.Multiaddr) (MulticastTransport, error) {
	m, err := resolveMulticastAddr(m)
	if err != nil {
		return nil, err
	}

	network, addr, err := manet.DialArgs(m)
	if err != nil {
		return nil, err
	}

	udpAddr, err := net.ResolveUDPAddr(network, addr)
	if err != nil {
		return nil, err
	}

	return &udpMulticastTransport{Group: udpAddr}, nil
}

type udpMulticastTransport struct {
	local atomic.Value // net.Addr
	Group *net.UDPAddr
}

func (u *udpMulticastTransport) DialMulticast() (Scatterer, error) {
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

func (u *udpMulticastTransport) ListenMulticast() (Gatherer, error) {
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

func newSender(d multicastDialer, outq <-chan outgoing) (io.Closer, error) {
	s, err := d.DialMulticast()
	if err != nil {
		return nil, err
	}

	// token-bucket rate limiter with burst factor of 8.
	bucket := make(chan struct{}, 8)
	for len(bucket) != cap(bucket) {
		bucket <- struct{}{}
	}

	// jittered ticker replenishes the token bucket
	ticker := jitterbug.New(time.Second, jitterbug.Uniform{
		Source: rand.New(rand.NewSource(time.Now().UnixNano())),
	})

	go func() {
		defer close(bucket)

		for range ticker.C {
			select {
			case bucket <- struct{}{}:
			default:
			}

		}
	}()

	// writer loop waits for a token to become available and
	// then sends a packet.
	go func() {
		defer ticker.Stop()

		for out := range outq {
			select {
			case <-out.C.Done():
			case <-bucket:
				out.Err <- s.Scatter(out.C, out.M)
			}
		}
	}()

	return s, nil
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

// multicastClient emits QUERY packets and awaits RESPONSE packet
// from peers.
type multicastClient struct {
	cq   chan struct{}
	qs   chan<- outgoing // queries
	add  chan addSink
	rm   chan func()
	recv *receiver
}

func newMulticastClient(conn multicastConn) multicastClient {
	return multicastClient{
		qs:   conn.out,
		recv: conn.receiver,
		cq:   make(chan struct{}),
		add:  make(chan addSink),
		rm:   make(chan func()),
	}
}

func (c multicastClient) FindPeers(ctx context.Context, ns string, opt []discovery.Option) (<-chan peer.AddrInfo, error) {
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

func (c multicastClient) options(opt []discovery.Option) (opts *discovery.Options, err error) {
	opts = &discovery.Options{}
	if err = opts.Apply(opt...); err == nil {
		if opts.Limit == 0 {
			opts.Limit = -1
		}
	}
	return
}

func (c multicastClient) addSink(m clientTopicManager, add addSink) {
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

func (c multicastClient) handleResponse(m clientTopicManager, p boot.MulticastPacket) error {
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

// multicastServer awaits QUERY packets and
// replies with RESPONSE packets
type multicastServer struct {
	log  log.Logger
	cq   chan struct{}
	ts   map[string]*serverTopic
	add  chan addServerTopic
	e    *record.Envelope
	recv *receiver
}

func (s multicastServer) Advertise(ctx context.Context, ns string, opt []discovery.Option) (time.Duration, error) {
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

func (s multicastServer) options(opt []discovery.Option) (opts *discovery.Options, err error) {
	opts = &discovery.Options{}
	if err = opts.Apply(opt...); err == nil {
		if opts.Ttl == 0 {
			opts.Ttl = peerstore.PermanentAddrTTL
		}
	}
	return
}

func (s multicastServer) handleAddTopic(ns string, ttl time.Duration) (err error) {
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

func (s multicastServer) setRecord(t *serverTopic) func() error {
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

func (s multicastServer) newServerTopic(ns string, stop func()) (*serverTopic, error) {
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
	log                log.Logger
	cq                 chan struct{}
	Queries, Responses chan boot.MulticastPacket
	c                  io.Closer
}

func newRecver(l multicastListener, opt []Option) (*receiver, error) {
	g, err := l.ListenMulticast()
	if err != nil {
		return nil, err
	}

	r := &receiver{
		cq:        make(chan struct{}),
		Queries:   make(chan boot.MulticastPacket, 8),
		Responses: make(chan boot.MulticastPacket, 8),
		c:         g,
	}

	for _, option := range opt {
		if err = option(r); err != nil {
			return nil, err
		}
	}

	go func() {
		defer close(r.Queries)
		defer close(r.Responses)

		var b = backoff.Backoff{
			Factor: 2,
			Jitter: true,
			Min:    time.Second,
			Max:    time.Minute * 5,
		}

		for {
			msg, err := g.Gather(context.Background())
			if err != nil {
				if ne, ok := err.(net.Error); !ok || !ne.Temporary() {
					return
				}

				r.log.WithError(err).
					WithField("backoff", b.ForAttempt(b.Attempt())).
					Debug("entering backoff state")

				select {
				case <-time.After(b.Duration()):
				case <-r.cq:
				}

				continue
			}

			b.Reset()

			if err = r.consume(msg); err != nil {
				r.log.WithError(err).Debug("malformed packet")
				return
			}
		}
	}()

	return r, nil
}

func (r *receiver) SetOption(v interface{}) (err error) {
	switch opt := v.(type) {
	case log.Logger:
		r.log = opt

	default:
		err = fmt.Errorf("invalid option '%s' for UDP multicast",
			reflect.TypeOf(v))
	}
	return
}

func (r receiver) consume(msg *capnp.Message) error {
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
		case <-r.cq:
		}
	}

	return err
}

func (r receiver) Close() error {
	close(r.cq)
	return r.c.Close()
}

type outgoing struct {
	C   context.Context
	M   *capnp.Message
	Err chan<- error
}

/*
 * P_MCAST
 */

// resolveMulticastAddr of form /multicast/<multiaddr>
func resolveMulticastAddr(m ma.Multiaddr) (resolved ma.Multiaddr, err error) {
	ma.ForEach(m, func(c ma.Component) bool {
		if c.Protocol().Code != P_MCAST {
			err = errors.New("invalid multicast addr")
		}

		resolved, err = ma.NewMultiaddrBytes(c.RawValue())
		return false
	})

	return
}

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
