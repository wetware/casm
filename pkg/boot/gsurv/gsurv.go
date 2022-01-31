package mudp

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
	cpGSurv "github.com/wetware/casm/internal/api/gsurv"
)

const (
	discLimit = 10
	discTTL   = time.Minute
	discDist  = uint8(255)

	timeout         = 5 * time.Second
	maxDatagramSize = 8192
)

type GSurv struct {
	h             host.Host
	mustFind      map[string]chan peer.AddrInfo
	mustAdvertise map[string]chan time.Duration
	mu            sync.Mutex

	c   comm
	err atomic.Value
}

func NewGSurv(h host.Host, addr net.Addr, opt ...Option) (gsurv *GSurv, err error) {
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

	gsurv = &GSurv{h: h, mustFind: make(map[string]chan peer.AddrInfo),
		mustAdvertise: make(map[string]chan time.Duration), c: c}

	go c.Listen(gsurv.multicastHandler)

	go func() {
		for {
			err := <-c.cherr
			if err != nil {
				gsurv.err.Store(err)
			} else {
				gsurv.err.Store(fmt.Errorf("closed"))
			}
			lconn.Close()
			dconn.Close()
			return
		}
	}()

	return gsurv, nil
}

func (gsurv *GSurv) Close() {
	gsurv.c.Close()
	for gsurv.err.Load() == nil {
	}
}

func (gsurv *GSurv) multicastHandler(n int, src net.Addr, buffer []byte) {
	msg, err := capnp.UnmarshalPacked(buffer[:n])
	if err != nil {
		return
	}

	root, err := cpGSurv.ReadRootGSurvPacket(msg)
	if err != nil {
		return
	}

	switch root.Which() {
	case cpGSurv.GSurvPacket_Which_request:
		request, err := root.Request()
		if err != nil {
			return
		}
		gsurv.handleMudpRequest(request)
	case cpGSurv.GSurvPacket_Which_response:
		response, err := root.Response()
		if err != nil {
			return
		}
		gsurv.handleMudpResponse(response)
	default:
	}
}

func (gsurv *GSurv) handleMudpRequest(request cpGSurv.GSurvRequest) {
	gsurv.mu.Lock()
	defer gsurv.mu.Unlock()

	// validate requester
	envelope, err := request.Src()
	if err != nil {
		return
	}

	var rec peer.PeerRecord
	if _, err = record.ConsumeTypedEnvelope(envelope, &rec); err != nil {
		return
	}
	if rec.PeerID == gsurv.h.ID() {
		return // request comes from itself
	}

	// check distance
	if err != nil {
		return
	}

	if dist([]byte(gsurv.h.ID()), []byte(rec.PeerID))>>uint32(request.Distance()) != 0 {
		return
	}

	ns, err := request.Namespace()
	if err != nil {
		return
	}

	if _, ok := gsurv.mustAdvertise[ns]; !ok {
		return
	}

	response, err := gsurv.buildResponse(ns)
	if err == nil {
		go gsurv.c.Send(response)
	}
}

func (gsurv *GSurv) buildResponse(ns string) ([]byte, error) {
	_, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		panic(err)
	}

	root, err := cpGSurv.NewRootGSurvPacket(seg)
	if err != nil {
		panic(err)
	}
	response, err := root.NewResponse()
	if err != nil {
		return nil, err
	}

	response.SetNamespace(ns)

	if cab, ok := peerstore.GetCertifiedAddrBook(gsurv.h.Peerstore()); ok {
		env := cab.GetPeerRecord(gsurv.h.ID())
		rec, err := env.Marshal()
		if err != nil {
			return nil, err
		}
		response.SetEnvelope(rec)
	}
	return root.Message().MarshalPacked()
}

func (gsurv *GSurv) handleMudpResponse(response cpGSurv.GSurvResponse) {
	var (
		finder chan peer.AddrInfo
		rec    peer.PeerRecord
		err    error
		ok     bool
	)

	gsurv.mu.Lock()
	defer gsurv.mu.Unlock()

	ns, err := response.Namespace()
	if err != nil {
		return
	}

	if finder, ok = gsurv.mustFind[ns]; !ok {
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

func (gsurv *GSurv) Advertise(ctx context.Context, ns string, opt ...discovery.Option) (time.Duration, error) {
	gsurv.mu.Lock()
	defer gsurv.mu.Unlock()

	opts, err := gsurv.options(ns, opt)
	if err != nil {
		return 0, err
	}

	if ttlChan, ok := gsurv.mustAdvertise[ns]; ok {
		ttlChan <- opts.Ttl
	} else {
		resetTtl := make(chan time.Duration)
		gsurv.mustAdvertise[ns] = resetTtl
		go gsurv.trackAdvertise(ns, resetTtl, opts.Ttl)
	}
	return opts.Ttl, nil
}

func (gsurv *GSurv) options(ns string, opt []discovery.Option) (opts *discovery.Options, err error) {
	opts = &discovery.Options{}
	if err = opts.Apply(opt...); err == nil && opts.Ttl == 0 {
		opts.Ttl = discTTL
	}

	return
}

func (gsurv *GSurv) trackAdvertise(ns string, resetTtl chan time.Duration, ttl time.Duration) {
	timer := time.NewTimer(ttl)
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			gsurv.mu.Lock()

			select { // check again TTL after acquiring lock
			case ttl := <-resetTtl:
				timer.Reset(ttl)
				gsurv.mu.Unlock()
			default:
				close(resetTtl)
				delete(gsurv.mustAdvertise, ns)
				gsurv.mu.Unlock()
				return
			}
		case ttl := <-resetTtl:
			timer.Reset(ttl)
		}
	}
}

func (gsurv *GSurv) FindPeers(ctx context.Context, ns string, opt ...discovery.Option) (<-chan peer.AddrInfo, error) {
	gsurv.mu.Lock()
	defer gsurv.mu.Unlock()

	var (
		opts *discovery.Options
		dist uint8
		err  error
	)

	opts, err = gsurv.options(ns, opt)
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

	request, err := gsurv.buildRequest(ns, dist)
	if err != nil {
		return nil, err
	}

	finder := make(chan peer.AddrInfo, opts.Limit)
	gsurv.mustFind[ns] = finder

	go gsurv.c.Send(request)
	go gsurv.closeFindPeers(ns, opts.Ttl)

	return finder, nil
}

func (gsurv *GSurv) closeFindPeers(ns string, ttl time.Duration) {
	timer := time.NewTimer(ttl)
	defer timer.Stop()

	<-timer.C

	gsurv.mu.Lock()
	defer gsurv.mu.Unlock()

	if finder, ok := gsurv.mustFind[ns]; ok {
		close(finder)
		delete(gsurv.mustFind, ns)
	}
}

func (gsurv *GSurv) buildRequest(ns string, dist uint8) ([]byte, error) {
	_, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		panic(err)
	}

	root, err := cpGSurv.NewRootGSurvPacket(seg)
	if err != nil {
		panic(err)
	}
	request, err := root.NewRequest()
	if err != nil {
		return nil, err
	}

	request.SetNamespace(ns)
	request.SetDistance(dist)
	if cab, ok := peerstore.GetCertifiedAddrBook(gsurv.h.Peerstore()); ok {
		env := cab.GetPeerRecord(gsurv.h.ID())
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

func dist(id1, id2 []byte) uint32 {
	xored := make([]byte, 4)
	for i := 0; i < 4; i++ {
		xored[i] = id1[len(id1)-i-1] ^ id2[len(id2)-i-1]
	}

	return binary.BigEndian.Uint32(xored)
}
