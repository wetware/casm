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
	discLimit = 10
	discTTL   = time.Minute
	discDist  = uint8(255)

	timeout         = 5 * time.Second
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

type Survey struct {
	ctx    context.Context
	cancel context.CancelFunc
	log    log.Logger

	advert         chan<- advert
	disc, discDone chan<- disc

	e   *record.Envelope
	rec *peer.PeerRecord

	mustFind      map[string]map[disc]struct{}
	mustAdvertise map[string]time.Time

	t   Transport
	c   comm
	err atomic.Value
}

func New(h host.Host, addr net.Addr, opt ...Option) (*Survey, error) {
	var (
		cq          = h.Network().Process().Closing()
		ctx, cancel = context.WithCancel(ctxutil.C(cq))

		cherr    = make(chan error)
		recv     = make(chan *capnp.Message, 8)
		send     = make(chan *capnp.Message, 8)
		advert   = make(chan advert)
		discover = make(chan disc)
		discDone = make(chan disc)
	)

	s := &Survey{
		ctx:           ctx,
		cancel:        cancel,
		advert:        advert,
		disc:          discover,
		discDone:      discDone,
		mustFind:      make(map[string]map[disc]struct{}),
		mustAdvertise: make(map[string]time.Time),
	}

	for _, option := range withDefaults(opt) {
		option(s)
	}

	lconn, err := s.t.Listen(addr)
	if err != nil {
		return nil, err
	}

	dconn, err := s.t.Dial(addr)
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

	sub, err := h.EventBus().Subscribe(new(event.EvtLocalAddressesUpdated))
	if err != nil {
		lconn.Close()
		dconn.Close()
		return nil, err
	}

	go func() {
		defer lconn.Close()
		defer dconn.Close()
		defer cancel()

		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		var t time.Time

		for {
			select {
			case ev := <-sub.Out():
				s.e = ev.(event.EvtLocalAddressesUpdated).SignedPeerRecord
				r, _ := s.e.Record()
				s.rec = r.(*peer.PeerRecord)

			case m := <-recv:
				if err := s.handleMessage(ctx, m); err != nil {
					s.log.WithError(err).Debug("dropped message")
				}

			case t = <-ticker.C:
				for ns, deadline := range s.mustAdvertise {
					if t.After(deadline) {
						delete(s.mustAdvertise, ns)
					}
				}

			case ad := <-advert:
				s.mustAdvertise[ad.NS] = t.Add(ad.TTL)

			case d := <-discover:
				nsm, ok := s.mustFind[d.NS]
				if !ok {
					nsm = map[disc]struct{}{}
					s.mustFind[d.NS] = nsm
				}

				nsm[d] = struct{}{}

			case d := <-discDone:
				if nsm, ok := s.mustFind[d.NS]; ok {
					delete(nsm, d)
				}
				close(d.Ch)

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

func (surv *Survey) Close() error {
	defer surv.cancel()
	err, _ := surv.err.Load().(error)
	return err
}

func (surv *Survey) handleMessage(ctx context.Context, m *capnp.Message) error {
	p, err := survey.ReadRootPacket(m)
	if err != nil {
		return err
	}

	switch p.Which() {
	case survey.Packet_Which_request:
		return surv.handleRequest(ctx, p)

	case survey.Packet_Which_response:
		return surv.handleResponse(ctx, p)
	}

	return fmt.Errorf("unrecognized packet type '%d'", p.Which())
}

func (surv *Survey) handleRequest(ctx context.Context, p survey.Packet) error {
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

	if rec.PeerID == surv.rec.PeerID {
		return nil // request comes from itself
	}

	if dist([]byte(surv.rec.PeerID), []byte(rec.PeerID))>>uint32(request.Distance()) != 0 {
		return nil // ignore
	}

	ns, err := p.Namespace()
	if err != nil {
		return err
	}

	if _, ok := surv.mustAdvertise[ns]; !ok {
		return nil // namespace not advertised
	}

	if err = surv.setResponse(ns, p); err != nil {
		return err
	}

	return surv.c.Send(ctx, p.Message())
}

func (surv *Survey) setResponse(ns string, p survey.Packet) error {
	if err := p.SetNamespace(ns); err != nil {
		return err
	}

	b, err := surv.e.Marshal()
	if err != nil {
		return err
	}

	return p.SetResponse(b)
}

func (surv *Survey) handleResponse(ctx context.Context, p survey.Packet) error {
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

	if finders, ok := surv.mustFind[ns]; ok {
		for f := range finders {
			select {
			case f.Ch <- &rec:
			default:
			}
		}
	}

	return nil
}

func (surv *Survey) Advertise(ctx context.Context, ns string, opt ...discovery.Option) (time.Duration, error) {
	var opts = discovery.Options{Ttl: discTTL}
	if err := opts.Apply(opt...); err != nil {
		return 0, err
	}

	select {
	case surv.advert <- advert{
		NS:  ns,
		TTL: opts.Ttl,
	}:
		return opts.Ttl, nil

	case <-ctx.Done():
		return 0, ctx.Err()

	case <-surv.ctx.Done():
		return 0, surv.ctx.Err()
	}
}

func (surv *Survey) FindPeers(ctx context.Context, ns string, opt ...discovery.Option) (<-chan peer.AddrInfo, error) {
	var opts discovery.Options
	if err := opts.Apply(opt...); err != nil {
		return nil, err
	}

	var (
		ok   bool
		dist uint8
		err  error
	)

	if dist, ok = opts.Other["distance"].(uint8); !ok {
		dist = discDist
	}

	m, err := surv.buildRequest(ns, dist)
	if err != nil {
		return nil, err
	}

	finder := make(chan *peer.PeerRecord, 8)
	out := make(chan peer.AddrInfo, 8)

	go func() {
		defer func() {
			select {
			case <-surv.ctx.Done():
			case surv.discDone <- disc{
				NS: ns,
				Ch: finder,
			}:
				close(out)
			}
		}()

		for {
			select {
			case rec, ok := <-finder:
				if !ok {
					return
				}

				select {
				case <-ctx.Done():
				case <-surv.ctx.Done():
				case out <- peer.AddrInfo{
					ID:    rec.PeerID,
					Addrs: rec.Addrs,
				}:
					if opts.Limit--; opts.Limit == 0 {
						return
					}
				}

			case <-ctx.Done():
				return
			case <-surv.ctx.Done():
				return
			}
		}
	}()

	select {
	case surv.disc <- disc{
		NS: ns,
		Ch: finder,
	}:

	case <-ctx.Done():
		return nil, ctx.Err()

	case <-surv.ctx.Done():
		return nil, errors.New("closed")
	}

	return out, surv.c.Send(ctx, m)
}

func (surv *Survey) buildRequest(ns string, dist uint8) (*capnp.Message, error) {
	_, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		panic(err)
	}

	p, err := survey.NewRootPacket(seg)
	if err != nil {
		panic(err)
	}

	request, err := p.NewRequest()
	if err != nil {
		return nil, err
	}

	p.SetNamespace(ns)
	request.SetDistance(dist)

	rec, err := surv.e.Marshal()
	if err == nil {
		err = request.SetSrc(rec)
	}

	return p.Message(), err
}

// func (surv *Survey) trackFindPeers(ns string, ttl time.Duration) {
// 	timer := time.NewTimer(ttl)
// 	defer timer.Stop()

// 	<-timer.C

// 	surv.mu.Lock()
// 	defer surv.mu.Unlock()

// 	if finder, ok := surv.mustFind[ns]; ok {
// 		close(finder)
// 		delete(surv.mustFind, ns)
// 	}
// }

func dist(id1, id2 []byte) uint32 {
	xored := make([]byte, 4)
	for i := 0; i < 4; i++ {
		xored[i] = id1[len(id1)-i-1] ^ id2[len(id2)-i-1]
	}

	return binary.BigEndian.Uint32(xored)
}

type advert struct {
	NS  string
	TTL time.Duration
}

type disc struct {
	NS string
	Ch chan<- *peer.PeerRecord
}
