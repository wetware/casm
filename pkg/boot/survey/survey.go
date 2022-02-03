package survey

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/cstockton/go-conv"
	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/libp2p/go-libp2p-core/record"
	cpSurvey "github.com/wetware/casm/internal/api/survey"
)

const (
	discLimit = 10
	discTTL   = time.Minute
	discDist  = uint8(255)

	timeout         = 5 * time.Second
	maxDatagramSize = 8192
)

type Survey struct {
	h             host.Host
	mustFind      map[string]chan peer.AddrInfo
	mustAdvertise map[string]chan time.Duration
	mu            sync.Mutex

	c   comm
	err atomic.Value
}

func NewSurvey(h host.Host, addr net.Addr, opt ...Option) (surv *Survey, err error) {
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

	c := comm{addr: addr, lconn: lconn, dconn: dconn, cherr: make(chan error)}

	surv = &Survey{h: h, mustFind: make(map[string]chan peer.AddrInfo),
		mustAdvertise: make(map[string]chan time.Duration), c: c}

	go c.Listen(surv.multicastHandler)

	go func() {
		for {
			err := <-c.cherr
			if err != nil {
				surv.err.Store(err)
			} else {
				surv.err.Store(fmt.Errorf("closed"))
			}
			lconn.Close()
			dconn.Close()
			return
		}
	}()

	return surv, nil
}

func (surv *Survey) Close() {
	if err := surv.err.Load(); err == nil {
		surv.c.Close()
		for surv.err.Load() == nil {
		}
	}
}

func (surv *Survey) multicastHandler(n int, src net.Addr, buffer []byte) {
	msg, err := capnp.UnmarshalPacked(buffer[:n])
	if err != nil {
		return
	}

	root, err := cpSurvey.ReadRootSurveyPacket(msg)
	if err != nil {
		return
	}

	switch root.Which() {
	case cpSurvey.SurveyPacket_Which_request:
		request, err := root.Request()
		if err != nil {
			return
		}
		surv.handleRequest(request)
	case cpSurvey.SurveyPacket_Which_response:
		response, err := root.Response()
		if err != nil {
			return
		}
		surv.handleResponse(response)
	default:
	}
}

func (surv *Survey) handleRequest(request cpSurvey.SurveyRequest) {
	surv.mu.Lock()
	defer surv.mu.Unlock()

	// validate requester
	envelope, err := request.Src()
	if err != nil {
		return
	}

	var rec peer.PeerRecord
	if _, err = record.ConsumeTypedEnvelope(envelope, &rec); err != nil {
		return
	}
	if rec.PeerID == surv.h.ID() {
		return // request comes from itself
	}

	if dist([]byte(surv.h.ID()), []byte(rec.PeerID))>>uint32(request.Distance()) != 0 {
		return
	}

	ns, err := request.Namespace()
	if err != nil {
		return
	}

	if _, ok := surv.mustAdvertise[ns]; !ok {
		return
	}

	response, err := surv.buildResponse(ns)
	if err == nil {
		go surv.c.Send(response)
	}
}

func (surv *Survey) buildResponse(ns string) ([]byte, error) {
	_, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		panic(err)
	}

	root, err := cpSurvey.NewRootSurveyPacket(seg)
	if err != nil {
		panic(err)
	}
	response, err := root.NewResponse()
	if err != nil {
		return nil, err
	}

	response.SetNamespace(ns)

	if cab, ok := peerstore.GetCertifiedAddrBook(surv.h.Peerstore()); ok {
		env := cab.GetPeerRecord(surv.h.ID())
		rec, err := env.Marshal()
		if err != nil {
			return nil, err
		}
		response.SetEnvelope(rec)
	}
	return root.Message().MarshalPacked()
}

func (surv *Survey) handleResponse(response cpSurvey.SurveyResponse) {
	var (
		finder chan peer.AddrInfo
		rec    peer.PeerRecord
		err    error
		ok     bool
	)

	surv.mu.Lock()
	defer surv.mu.Unlock()

	ns, err := response.Namespace()
	if err != nil {
		return
	}

	if finder, ok = surv.mustFind[ns]; !ok {
		return
	}

	envelope, err := response.Envelope()
	if err != nil {
		return
	}

	if _, err = record.ConsumeTypedEnvelope(envelope, &rec); err != nil {
		return
	}
	select {
	case finder <- peer.AddrInfo{ID: rec.PeerID, Addrs: rec.Addrs}:
	default:
		return
	}
}

func (surv *Survey) Advertise(ctx context.Context, ns string, opt ...discovery.Option) (time.Duration, error) {
	if err := surv.err.Load(); err != nil {
		return 0, err.(error)
	}

	surv.mu.Lock()
	defer surv.mu.Unlock()

	opts, err := surv.options(ns, opt)
	if err != nil {
		return 0, err
	}

	if ttlChan, ok := surv.mustAdvertise[ns]; ok {
		ttlChan <- opts.Ttl
	} else {
		resetTtl := make(chan time.Duration)
		surv.mustAdvertise[ns] = resetTtl
		go surv.trackAdvertise(ns, resetTtl, opts.Ttl)
	}
	return opts.Ttl, nil
}

func (surv *Survey) options(ns string, opt []discovery.Option) (opts *discovery.Options, err error) {
	opts = &discovery.Options{}
	if err = opts.Apply(opt...); err == nil && opts.Ttl == 0 {
		opts.Ttl = discTTL
	}

	return
}

func (surv *Survey) trackAdvertise(ns string, resetTtl chan time.Duration, ttl time.Duration) {
	timer := time.NewTimer(ttl)
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			surv.mu.Lock()

			select { // check again TTL after acquiring lock
			case ttl := <-resetTtl:
				timer.Reset(ttl)
				surv.mu.Unlock()
			default:
				close(resetTtl)
				delete(surv.mustAdvertise, ns)
				surv.mu.Unlock()
				return
			}
		case ttl := <-resetTtl:
			timer.Reset(ttl)
		}
	}
}

func (surv *Survey) FindPeers(ctx context.Context, ns string, opt ...discovery.Option) (<-chan peer.AddrInfo, error) {
	if err := surv.err.Load(); err != nil {
		return nil, err.(error)
	}

	surv.mu.Lock()
	defer surv.mu.Unlock()

	var (
		opts *discovery.Options
		dist uint8
		err  error
	)

	opts, err = surv.options(ns, opt)
	if err != nil {
		return nil, err
	}

	if b, ok := opts.Other["distance"]; ok {
		dist, err = conv.Uint8(b)
		if err != nil {
			return nil, err
		}
	} else {
		dist = discDist
	}

	request, err := surv.buildRequest(ns, dist)
	if err != nil {
		return nil, err
	}

	finder := make(chan peer.AddrInfo, opts.Limit)
	surv.mustFind[ns] = finder

	go surv.c.Send(request)
	go surv.trackFindPeers(ns, opts.Ttl)

	return finder, nil
}

func (surv *Survey) buildRequest(ns string, dist uint8) ([]byte, error) {
	_, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		panic(err)
	}

	root, err := cpSurvey.NewRootSurveyPacket(seg)
	if err != nil {
		panic(err)
	}
	request, err := root.NewRequest()
	if err != nil {
		return nil, err
	}

	request.SetNamespace(ns)
	request.SetDistance(dist)
	if cab, ok := peerstore.GetCertifiedAddrBook(surv.h.Peerstore()); ok {
		env := cab.GetPeerRecord(surv.h.ID())
		rec, err := env.Marshal()
		if err != nil {
			return nil, err
		}
		request.SetSrc(rec)
	} else {
		return nil, errors.New("unable to get certified address book from libp2p host")
	}
	return root.Message().MarshalPacked()
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
