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
	"github.com/cstockton/go-conv"
	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/record"
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

type Survey struct {
	ctx    context.Context
	cancel context.CancelFunc
	advert chan<- advert

	e   *record.Envelope
	rec *peer.PeerRecord

	mustFind      map[string]chan peer.AddrInfo
	mustAdvertise map[string]time.Time

	c   comm
	err atomic.Value
}

func New(h host.Host, addr net.Addr, opt ...Option) (*Survey, error) {
	config := Config{}
	config.Apply(opt)

	lconn, err := config.Listen(addr)
	if err != nil {
		return nil, err
	}

	dconn, err := config.Dial(addr)
	if err != nil {
		lconn.Close()
		return nil, err
	}

	var (
		cherr = make(chan error)
		recv  = make(chan *capnp.Message, 8)
		send  = make(chan *capnp.Message, 8)
	)

	c := comm{
		cherr: cherr,
		recv:  recv,
		send:  send,

		addr:  addr,
		dconn: dconn,
	}

	var (
		advert = make(chan advert)

		cq          = h.Network().Process().Closing()
		ctx, cancel = context.WithCancel(ctxutil.C(cq))
	)

	surv := &Survey{
		ctx:           ctx,
		cancel:        cancel,
		c:             c,
		advert:        advert,
		mustFind:      make(map[string]chan peer.AddrInfo),
		mustAdvertise: make(map[string]time.Time),
	}

	go c.StartRecv(ctx, lconn)
	go c.StartSend(ctx, dconn)

	sub, err := h.EventBus().Subscribe(new(event.EvtLocalAddressesUpdated))
	if err != nil {
		lconn.Close()
		dconn.Close()
		return nil, err
	}

	go func() {
		defer cancel()

		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		var t time.Time

		for {
			select {
			case ev := <-sub.Out():
				surv.e = ev.(event.EvtLocalAddressesUpdated).SignedPeerRecord
				r, _ := surv.e.Record()
				surv.rec = r.(*peer.PeerRecord)

			case m := <-recv:
				if err := surv.handleMessage(ctx, m); err != nil {
					// TODO:  log
				}

			case t = <-ticker.C:
				for ns, deadline := range surv.mustAdvertise {
					if t.After(deadline) {
						delete(surv.mustAdvertise, ns)
					}
				}

			case ad := <-advert:
				surv.mustAdvertise[ad.NS] = t.Add(ad.TTL)

			case err := <-cherr:
				if err == nil {
					err = errors.New("closed")
				}

				surv.err.Store(err)
				return

			case <-ctx.Done():
				surv.err.Store(errors.New("host closed"))
			}
		}
	}()

	return surv, nil
}

func (surv *Survey) Close() error {
	defer surv.cancel()
	err, _ := surv.err.Load().(error)
	return err
}

func (surv *Survey) handleMessage(ctx context.Context, m *capnp.Message) error {
	p, err := survey.ReadRootSurveyPacket(m)
	if err != nil {
		return err
	}

	switch p.Which() {
	case survey.SurveyPacket_Which_request:
		return surv.handleRequest(ctx, p)

	case survey.SurveyPacket_Which_response:
		return surv.handleResponse(ctx, p)
	}

	return fmt.Errorf("unrecognized packet type '%d'", p.Which())
}

func (surv *Survey) handleRequest(ctx context.Context, p survey.SurveyPacket) error {
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

	ns, err := request.Namespace()
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

func (surv *Survey) setResponse(ns string, p survey.SurveyPacket) error {
	response, err := p.NewResponse()
	if err != nil {
		return err
	}

	if err = response.SetNamespace(ns); err != nil {
		return err
	}

	b, err := surv.e.Marshal()
	if err != nil {
		return err
	}

	return response.SetEnvelope(b)
}

func (surv *Survey) handleResponse(ctx context.Context, p survey.SurveyPacket) error {
	response, err := p.Response()
	if err != nil {
		return err
	}

	var (
		finder chan peer.AddrInfo
		rec    peer.PeerRecord
		ok     bool
	)

	ns, err := response.Namespace()
	if err != nil {
		return err
	}

	if finder, ok = surv.mustFind[ns]; !ok {
		return nil
	}

	envelope, err := response.Envelope()
	if err != nil {
		return err
	}

	if _, err = record.ConsumeTypedEnvelope(envelope, &rec); err != nil {
		return err
	}

	select {
	case finder <- peer.AddrInfo{ID: rec.PeerID, Addrs: rec.Addrs}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
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
		dist uint8
		err  error
	)

	if b, ok := opts.Other["distance"]; ok {
		dist, err = conv.Uint8(b)
		if err != nil {
			return nil, err
		}
	} else {
		dist = discDist
	}

	m, err := surv.buildRequest(ns, dist)
	if err != nil {
		return nil, err
	}

	// XXX:  YOU ARE HERE

	finder := make(chan peer.AddrInfo, opts.Limit)
	surv.mustFind[ns] = finder

	if err = surv.c.Send(ctx, m); err != nil {
		return nil, err
	}

	go surv.trackFindPeers(ns, opts.Ttl)

	return finder, nil
}

func (surv *Survey) buildRequest(ns string, dist uint8) (*capnp.Message, error) {
	_, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		panic(err)
	}

	p, err := survey.NewRootSurveyPacket(seg)
	if err != nil {
		panic(err)
	}

	request, err := p.NewRequest()
	if err != nil {
		return nil, err
	}

	request.SetNamespace(ns)
	request.SetDistance(dist)

	if rec, err := surv.e.Marshal(); err == nil {
		err = request.SetSrc(rec)
	}

	return p.Message(), err
}

func (surv *Survey) trackFindPeers(ns string, ttl time.Duration) {
	timer := time.NewTimer(ttl)
	defer timer.Stop()

	<-timer.C

	surv.mu.Lock()
	defer surv.mu.Unlock()

	if finder, ok := surv.mustFind[ns]; ok {
		close(finder)
		delete(surv.mustFind, ns)
	}
}

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
