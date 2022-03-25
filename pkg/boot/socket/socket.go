package socket

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/record"
	"github.com/wetware/casm/internal/api/boot"
	"go.uber.org/multierr"
)

// TODO:  can we reduce this?
const maxDatagramSize = 8 << 10 // 8 KB

var ErrClosed = errors.New("closed")

type Validator func(*record.Envelope, *Record) error

type Protocol struct {
	HandleError   func(error)
	HandleRequest func(Request, net.Addr)
	Validate      Validator
	Cache         *RecordCache
}

type Socket struct {
	sock socket
	tick *time.Ticker

	mu   sync.RWMutex
	subs map[string]subscriberSet
	advt map[string]time.Time
	time time.Time
	sub  event.Subscription

	cache *RecordCache
}

func New(conn net.PacketConn, p Protocol) *Socket {
	sock := &Socket{
		sock:  socket{conn},
		subs:  make(map[string]subscriberSet),
		advt:  make(map[string]time.Time),
		tick:  time.NewTicker(time.Millisecond * 500),
		cache: p.Cache,
	}

	go sock.tickloop()
	go sock.scanloop(p)

	return sock
}

func (s *Socket) Close() (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.tick.Stop()

	if err = s.sock.Close(); s.sub != nil {
		err = multierr.Combine(err, s.sub.Close())
	}

	return
}

func (s *Socket) Tracking(ns string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, ok := s.advt[ns]
	return ok
}

func (s *Socket) Track(ctx context.Context, h host.Host, ns string, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	err := s.ensureTrackHostAddr(ctx, h)
	if err != nil {
		s.advt[ns] = s.time.Add(ttl)
	}

	return err
}

// LocalAddr returns the local address on which the socket is listening.
// If s is a multicast socket, the returned address is that of the multicast
// group.
//
// The returned net.Addr is shared across calls, and MUST NOT be modified.
func (s *Socket) LocalAddr() net.Addr { return s.sock.LocalAddr() }

func (s *Socket) Send(e *record.Envelope, addr net.Addr) error {
	return s.sock.Send(e, addr)
}

func (s *Socket) Subscribe(ns string, limit int) (<-chan peer.AddrInfo, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ss, ok := s.subs[ns]
	if !ok {
		ss = make(subscriberSet)
		s.subs[ns] = ss
	}

	ch := make(chan peer.AddrInfo, bufsize(limit))
	cancel := func() {
		s.mu.Lock()
		defer s.mu.Unlock()

		if ss.Remove(ch) {
			delete(s.subs, ns)
		}
	}

	ss.Add(ch, limiter(limit, cancel))

	return ch, cancel
}

// caller MUST hold mu.
func (s *Socket) ensureTrackHostAddr(ctx context.Context, h host.Host) (err error) {
	// previously initialized?
	if s.sub != nil {
		return
	}

	// client host?  (best-effort)
	//
	// The caller may not have set addresses on the host yet, so
	// we should allow them to retry. Therefore, we perform this
	// check outside of sync.Once.
	if len(h.Addrs()) == 0 {
		return errors.New("host not accepting connections")
	}

	s.sub, err = h.EventBus().Subscribe(new(event.EvtLocalAddressesUpdated))
	if err != nil {
		return
	}

	// Ensure a sync operation is run before continuing the call
	// to Advertise, as this may otherwise cause a panic.
	//
	// The host may have unregistered its addresses concurrently
	// with the call to Subscribe, so we provide a cancellation
	// mechanism via the context.  Note that if the first call to
	// ensureTrackHostAddr fails, subsequent calls will also fail.
	select {
	case v, ok := <-s.sub.Out():
		if !ok {
			return fmt.Errorf("host %w", ErrClosed)
		}

		evt := v.(event.EvtLocalAddressesUpdated)
		s.cache.Reset(evt.SignedPeerRecord)

	case <-ctx.Done():
		s.sub.Close()
		return ctx.Err()
	}

	go func() {
		for v := range s.sub.Out() {
			evt := v.(event.EvtLocalAddressesUpdated)
			s.cache.Reset(evt.SignedPeerRecord)
		}
	}()

	return err
}

func (s *Socket) tickloop() {
	for t := range s.tick.C {
		s.mu.Lock()

		s.time = t
		for ns, deadline := range s.advt {
			if t.After(deadline) {
				delete(s.advt, ns)
			}
		}

		s.mu.Unlock()
	}
}

func (s *Socket) scanloop(p Protocol) {
	var (
		r    Record
		addr net.Addr
		err  error
	)

	for {
		if addr, err = s.sock.Scan(p.Validate, &r); err != nil {
			p.HandleError(err)

			// socket closed?
			if ne, ok := err.(net.Error); ok && !ne.Timeout() {
				return
			}

			continue
		}

		// Packet is already validated.  It's either a response, or
		// some sort of request.
		switch boot.Packet(r).Which() {
		case boot.Packet_Which_request:
			p.HandleRequest(Request{r}, addr)

		default:
			if err := s.dispatch(Response{r}, addr); err != nil {
				p.HandleError(err)
			}
		}
	}
}

func (s *Socket) dispatch(r Response, addr net.Addr) error {
	ns, err := r.Namespace()
	if err != nil {
		return err
	}

	var info peer.AddrInfo
	if err = r.Bind(&info); err != nil {
		return err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if ss, ok := s.subs[ns]; ok {
		for sub, lim := range ss {
			select {
			case sub.Out <- info:
				lim.Decr()

			default:
			}
		}
	}

	return err
}

// BasicValidator returns a validator that checks that the
// host corresponding to id is not the packet's originator,
// and that that the envelope was signed by the peer whose
// ID appears in the packet.
func BasicValidator(id peer.ID) Validator {
	return func(e *record.Envelope, r *Record) error {
		peer, err := r.PeerID()
		if err != nil {
			return err
		}

		// originates from local host?
		if peer == id {
			return errors.New("same-host request")
		}

		// envelope was signed by peer?
		if !peer.MatchesPublicKey(e.PublicKey) {
			err = errors.New("envelope not signed by peer")
		}

		return err
	}
}

// Socket is a a packet-oriented network interface that exchanges
// signed messages.
//
// The wrapped PacketConn implementation MUST flush its send buffer
// in a timely manner (as is common in packet-oriented transports
// like UDP).  It must also provide unreliable delivery semantics;
// if the underlying transport is reliable, it MUST suppress any
// errors due to failed connections or delivery.
type socket struct{ net.PacketConn }

// Send writes the message m to addr.  Send does not support
// write timeouts since the underlying PacketConn provides a
// best-effort transmission semantics, and flushes its buffer
// quickly.
//
// Implementations MUST support concurrent calls to Send.
func (s socket) Send(e *record.Envelope, addr net.Addr) error {
	b, err := e.Marshal()
	if err == nil {
		_, err = s.WriteTo(b, addr)
	}

	return err
}

// Scan a record from the from the Socket, returning the originator's
// along with the signed envelope.
//
// Callers MUST NOT make concurrent calls to Recv.
func (s socket) Scan(validate Validator, p *Record) (net.Addr, error) {
	var buf [maxDatagramSize]byte
	n, addr, err := s.ReadFrom(buf[:])
	if err != nil {
		return nil, err
	}

	e, err := record.ConsumeTypedEnvelope(buf[:n], p)
	if err != nil {
		return nil, err
	}

	return addr, validate(e, p)
}

type subscriberSet map[subscriber]*resultLimiter

type resultLimiter struct {
	remaining int32
	cancel    func()
}

func limiter(limit int, cancel func()) (l *resultLimiter) {
	if limit > 0 {
		l = &resultLimiter{
			remaining: int32(limit),
			cancel:    cancel,
		}
	}

	return
}

func (l *resultLimiter) Decr() {
	if l != nil && atomic.AddInt32(&l.remaining, -1) == 0 {
		go l.cancel()
	}
}

func bufsize(limit int) int {
	if limit < 0 {
		return 0
	}

	return limit
}

type subscriber struct{ Out chan<- peer.AddrInfo }

func (ss subscriberSet) Add(s chan<- peer.AddrInfo, l *resultLimiter) {
	ss[subscriber{s}] = l
}

func (ss subscriberSet) Remove(s chan<- peer.AddrInfo) (empty bool) {
	delete(ss, subscriber{s})
	return len(ss) == 0
}

type thunk func(time.Time)
