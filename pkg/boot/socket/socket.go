// Package socket implements signed sockets for bootstrap services.
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
	"go.uber.org/multierr"
)

const maxDatagramSize = 2 << 10 // KB

var ErrClosed = errors.New("closed")

type Protocol struct {
	HandleError   func(error)
	HandleRequest func(Request, net.Addr)
	Validate      Validator
	RateLimiter   *RateLimiter
}

// Socket is a a packet-oriented network interface that exchanges
// signed messages.
type Socket struct {
	sock packetConn
	tick *time.Ticker

	mu   sync.RWMutex
	subs map[string]subscriberSet
	advt map[string]time.Time
	time time.Time
	sub  event.Subscription

	cache *RecordCache

	prot Protocol
}

// New socket.  The wrapped PacketConn implementation MUST flush
// its send buffer in a timely manner.  It must also provide
// unreliable delivery semantics; if the underlying transport is
// reliable, it MUST suppress any errors due to failed connections
// or delivery.  The standard net.PacketConn implementations satisfy
// these condiitions
func New(conn net.PacketConn, p Protocol) *Socket {
	sock := &Socket{
		sock: newPacketConn(conn, p.RateLimiter),
		subs: make(map[string]subscriberSet),
		advt: make(map[string]time.Time),
		time: time.Now(),
		tick: time.NewTicker(time.Millisecond * 500),
		prot: p,
	}

	return sock
}

func (s *Socket) Start() {
	go s.tickloop()
	go s.scanloop()
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

func (s *Socket) Track(ctx context.Context, h host.Host, ns string, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.advt[ns] = s.time.Add(ttl)
}

// LocalAddr returns the local address on which the socket is listening.
// If s is a multicast socket, the returned address is that of the multicast
// group.
//
// The returned net.Addr is shared across calls, and MUST NOT be modified.
func (s *Socket) LocalAddr() net.Addr { return s.sock.LocalAddr() }

func (s *Socket) Send(ctx context.Context, e *record.Envelope, addr net.Addr) error {
	return s.sock.Send(ctx, e, addr)
}

func (s *Socket) Subscribe(ns string, limit int) (<-chan peer.AddrInfo, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ss, ok := s.subs[ns]
	if !ok {
		ss = make(subscriberSet)
		s.subs[ns] = ss
	}

	var (
		once sync.Once
		ch   = make(chan peer.AddrInfo, bufsize(limit))
	)

	cancel := func() {
		once.Do(func() {
			defer close(ch)

			s.mu.Lock()
			defer s.mu.Unlock()

			if ss.Remove(ch) {
				delete(s.subs, ns)
			}
		})
	}

	ss.Add(ch, limiter(limit, cancel))

	return ch, cancel
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

func (s *Socket) scanloop() {
	var (
		r    Record
		addr net.Addr
		err  error
	)

	for {
		if addr, err = s.sock.Scan(s.prot.Validate, &r); err != nil {
			s.prot.HandleError(err)

			// socket closed?
			if ne, ok := err.(net.Error); ok && !ne.Timeout() {
				return
			}

			continue
		}

		// Packet is already validated.  It's either a response, or
		// some sort of request.
		switch r.Type() {
		case TypeResponse:
			if err := s.dispatch(Response{r}, addr); err != nil {
				s.prot.HandleError(err)
			}

		case TypeRequest, TypeSurvey:
			s.prot.HandleRequest(Request{r}, addr)

		default:
			s.prot.HandleError(fmt.Errorf("%s: invalid packet type", r.Type()))
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

	// release any subscriptions that have reached their limit.
	var done []func()
	defer func() {
		for _, release := range done {
			release()
		}
	}()

	s.mu.RLock()
	defer s.mu.RUnlock()

	if ss, ok := s.subs[ns]; ok {
		for sub, lim := range ss {
			select {
			case sub.Out <- info:
				if lim.Decr() {
					done = append(done, lim.cancel)
				}

			default:
			}
		}
	}

	return err
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

func (l *resultLimiter) Decr() bool {
	return l != nil && atomic.AddInt32(&l.remaining, -1) == 0
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
