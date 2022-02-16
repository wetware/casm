package survey

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync/atomic"
	"time"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/record"
	"github.com/lthibault/log"
	ctxutil "github.com/lthibault/util/ctx"
	"github.com/wetware/casm/internal/api/survey"
)

const (
	defaultTTL      = time.Minute
	maxDatagramSize = 8192
)

type DialFunc func(net.Addr) (net.PacketConn, error)

func (dial DialFunc) Dial(addr net.Addr) (net.PacketConn, error) {
	if dial != nil {
		return dial(addr)
	}

	udpAddr, err := net.ResolveUDPAddr(addr.Network(), addr.String())
	if err != nil {
		return nil, err
	}
	return net.ListenUDP(addr.Network(), udpAddr)
}

type ListenFunc func(net.Addr) (net.PacketConn, error)

func (listen ListenFunc) Listen(addr net.Addr) (net.PacketConn, error) {
	if listen != nil {
		return listen(addr)
	}

	udpAddr, err := net.ResolveUDPAddr(addr.Network(), addr.String())
	if err != nil {
		return nil, err
	}
	return net.ListenMulticastUDP(addr.Network(), nil, udpAddr)
}

type Transport struct {
	DialFunc
	ListenFunc
}

type Surveyor struct {
	ctx    context.Context
	cancel context.CancelFunc
	log    log.Logger

	t     time.Time
	thunk chan<- func()

	e   atomic.Value
	rec atomic.Value

	mustFind      map[string]map[listener]struct{}
	mustAdvertise map[string]time.Time

	tp  Transport
	c   comm
	err atomic.Value
}

func New(h host.Host, addr net.Addr, opt ...Option) (*Surveyor, error) {
	var (
		cq          = h.Network().Process().Closing()
		ctx, cancel = context.WithCancel(ctxutil.C(cq))

		cherr = make(chan error)
		thunk = make(chan func())
		recv  = make(chan *capnp.Message, 8)
		send  = make(chan *capnp.Message, 8)
	)

	s := &Surveyor{
		ctx:           ctx,
		cancel:        cancel,
		t:             time.Now(),
		thunk:         thunk,
		mustFind:      make(map[string]map[listener]struct{}),
		mustAdvertise: make(map[string]time.Time),
	}

	for _, option := range withDefaults(opt) {
		option(s)
	}

	sub, err := h.EventBus().Subscribe(new(event.EvtLocalAddressesUpdated))
	if err != nil {
		return nil, err
	}

	// Ensure the sync operation is run before anything else
	select {
	case v := <-sub.Out():
		s.setLocalRecord(v.(event.EvtLocalAddressesUpdated))

	case <-ctx.Done():
		return nil, ctx.Err()
	}

	lconn, err := s.tp.Listen(addr)
	if err != nil {
		return nil, err
	}

	dconn, err := s.tp.Dial(addr)
	if err != nil {
		lconn.Close()
		return nil, err
	}

	s.c = comm{
		log:   s.log,
		cherr: cherr,
		recv:  recv,
		send:  send,

		addr:  addr,
		dconn: dconn,
	}

	go s.c.StartRecv(ctx, lconn)
	go s.c.StartSend(ctx, dconn)

	go func() {
		for v := range sub.Out() {
			s.setLocalRecord(v.(event.EvtLocalAddressesUpdated))
		}
	}()

	go func() {
		defer lconn.Close()
		defer dconn.Close()
		defer sub.Close()
		defer cancel()

		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case s.t = <-ticker.C:
				for ns, deadline := range s.mustAdvertise {
					if s.t.After(deadline) {
						delete(s.mustAdvertise, ns)
					}
				}

			case f := <-thunk:
				f()
			case m := <-recv:
				if err := s.handleMessage(ctx, m); err != nil {
					s.log.WithError(err).Debug("dropped message")
				}

			case err := <-cherr:
				if err == nil {
					err = errors.New("closed")
				}

				s.err.Store(err)
				return

			case <-ctx.Done():
				s.err.Store(errors.New("host closed"))
			}
		}
	}()

	return s, nil
}

func (s *Surveyor) Close() error {
	defer s.cancel()
	err, _ := s.err.Load().(error)
	return err
}

func (s *Surveyor) setLocalRecord(ev event.EvtLocalAddressesUpdated) {
	r, _ := ev.SignedPeerRecord.Record()
	rec := r.(*peer.PeerRecord)
	s.e.Store(ev.SignedPeerRecord)
	s.rec.Store(rec)
}

func (s *Surveyor) handleMessage(ctx context.Context, m *capnp.Message) error {
	p, err := survey.ReadRootPacket(m)
	if err != nil {
		return err
	}

	switch p.Which() {
	case survey.Packet_Which_request:
		return s.handleRequest(ctx, p)

	case survey.Packet_Which_response:
		return s.handleResponse(ctx, p)
	}

	return fmt.Errorf("unrecognized packet type '%d'", p.Which())
}

func (s *Surveyor) handleRequest(ctx context.Context, p survey.Packet) error {
	request, err := p.Request()
	if err != nil {
		return err
	}

	// validate requester
	envelope, err := request.Src()
	if err != nil {
		return err
	}

	var rec peer.PeerRecord
	if _, err = record.ConsumeTypedEnvelope(envelope, &rec); err != nil {
		return err
	}

	if rec.PeerID == s.rec.Load().(*peer.PeerRecord).PeerID {
		return nil // request comes from itself
	}

	if s.ignore(rec.PeerID, request.Distance()) {
		return nil // ignore
	}

	ns, err := p.Namespace()
	if err != nil {
		return err
	}

	if _, ok := s.mustAdvertise[ns]; !ok {
		return nil // namespace not advertised
	}

	if err = s.setResponse(ns, p); err != nil {
		return err
	}

	return s.c.Send(ctx, p.Message())
}

func (s *Surveyor) ignore(id peer.ID, d uint8) bool {
	return xor(s.rec.Load().(*peer.PeerRecord).PeerID, id)>>uint32(d) != 0
}

func (s *Surveyor) setResponse(ns string, p survey.Packet) error {
	if err := p.SetNamespace(ns); err != nil {
		return err
	}

	b, err := s.e.Load().(*record.Envelope).Marshal()
	if err != nil {
		return err
	}

	return p.SetResponse(b)
}

func (s *Surveyor) handleResponse(ctx context.Context, p survey.Packet) error {
	ns, err := p.Namespace()
	if err != nil {
		return err
	}

	envelope, err := p.Response()
	if err != nil {
		return err
	}

	var rec peer.PeerRecord
	if _, err = record.ConsumeTypedEnvelope(envelope, &rec); err != nil {
		return err
	}

	if finders, ok := s.mustFind[ns]; ok {
		for f := range finders {
			select {
			case f.Ch <- &rec:
			default:
			}
		}
	}

	return nil
}

func (s *Surveyor) Advertise(ctx context.Context, ns string, opt ...discovery.Option) (time.Duration, error) {
	var opts = discovery.Options{Ttl: defaultTTL}
	if err := opts.Apply(opt...); err != nil {
		return 0, err
	}

	select {
	case s.thunk <- func() { s.mustAdvertise[ns] = s.t.Add(opts.Ttl) }:
		return opts.Ttl, nil

	case <-ctx.Done():
		return 0, ctx.Err()

	case <-s.ctx.Done():
		return 0, s.ctx.Err()
	}
}

func (s *Surveyor) FindPeers(ctx context.Context, ns string, opt ...discovery.Option) (<-chan peer.AddrInfo, error) {
	var opts discovery.Options
	if err := opts.Apply(opt...); err != nil {
		return nil, err
	}

	m, err := s.buildRequest(ns, distance(opts))
	if err != nil {
		return nil, err
	}

	var (
		cherr = make(chan error, 1) // TODO:  pool?
		recv  = make(chan *peer.PeerRecord, 8)
		out   = make(chan peer.AddrInfo, 8)
	)

	loop := func() {
		defer close(out)
		defer func() {
			select {
			case s.thunk <- func() {
				if nsm, ok := s.mustFind[ns]; ok {
					delete(nsm, listener{recv})
				}
				close(recv)
			}:
			case <-s.ctx.Done():
			}
		}()

		for {
			select {
			case rec, ok := <-recv:
				if !ok {
					return
				}

				select {
				case <-ctx.Done():
				case <-s.ctx.Done():
				case out <- peer.AddrInfo{ID: rec.PeerID, Addrs: rec.Addrs}:
					if opts.Limit--; opts.Limit == 0 {
						return
					}
				}

			case <-ctx.Done():
				return
			case <-s.ctx.Done():
				return
			}
		}
	}

	findPeers := func() {
		if err := s.c.Send(ctx, m); err != nil {
			cherr <- err
			return
		}

		ls, ok := s.mustFind[ns]
		if !ok {
			ls = map[listener]struct{}{}
			s.mustFind[ns] = ls
		}

		ls[listener{recv}] = struct{}{}
		cherr <- nil
		go loop()
	}

	select {
	case s.thunk <- findPeers:
	case <-ctx.Done():
		return nil, ctx.Err()

	case <-s.ctx.Done():
		return nil, errors.New("closed")
	}

	select {
	case err := <-cherr:
		return out, err

	case <-ctx.Done():
		return nil, ctx.Err()

	case <-s.ctx.Done():
		return nil, errors.New("closed")
	}
}

func (s *Surveyor) buildRequest(ns string, dist uint8) (*capnp.Message, error) {
	_, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		return nil, err
	}

	p, err := survey.NewRootPacket(seg)
	if err != nil {
		return nil, err
	}

	request, err := p.NewRequest()
	if err != nil {
		return nil, err
	}

	if err = p.SetNamespace(ns); err != nil {
		return nil, err
	}

	request.SetDistance(dist)

	rec, err := s.e.Load().(*record.Envelope).Marshal()
	if err == nil {
		err = request.SetSrc(rec)
	}

	return p.Message(), err
}

func xor(id1, id2 peer.ID) uint32 {
	xored := make([]byte, 4)
	for i := 0; i < 4; i++ {
		xored[i] = id1[len(id1)-i-1] ^ id2[len(id2)-i-1]
	}

	return binary.BigEndian.Uint32(xored)
}

type listener struct {
	Ch chan<- *peer.PeerRecord
}
