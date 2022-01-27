package mudp

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"sync"
	"time"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/cstockton/go-conv"
	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/libp2p/go-libp2p-core/record"
	cpMudp "github.com/wetware/casm/internal/api/mudp"
)

const (
	multicastAddr = "224.0.1.241:3037"

	discLimit = 10
	discTTL   = time.Minute
	discDist  = uint8(255)

	timeout = 5 * time.Second
)

type Mudp struct {
	h             host.Host
	mc            *multicaster
	mustFind      map[string]chan peer.AddrInfo
	mustAdvertise map[string]chan time.Duration
	mu            sync.Mutex
}

func NewMudp(h host.Host, opt ...Option) (mudp *Mudp, err error) {
	config := Config{}
	config.Apply(opt)

	mc, err := NewMulticaster(config.Addr, config.Dial, config.Listen)
	if err != nil {
		return
	}

	mudp = &Mudp{h: h, mc: mc, mustFind: make(map[string]chan peer.AddrInfo),
		mustAdvertise: make(map[string]chan time.Duration)}
	ready := make(chan bool)
	go mc.Listen(ready, mudp.multicastHandler)
	<-ready

	return mudp, nil
}

func (mudp *Mudp) Close() {
	mudp.mc.Close()
}

func (mudp *Mudp) multicastHandler(n int, src net.Addr, buffer []byte) {
	msg, err := capnp.UnmarshalPacked(buffer[:n])
	if err != nil {
		return
	}

	root, err := cpMudp.ReadRootMudpPacket(msg)
	if err != nil {
		return
	}

	switch root.Which() {
	case cpMudp.MudpPacket_Which_request:
		request, err := root.Request()
		if err != nil {
			return
		}
		mudp.handleMudpRequest(request)
	case cpMudp.MudpPacket_Which_response:
		response, err := root.Response()
		if err != nil {
			return
		}
		mudp.handleMudpResponse(response)
	default:
	}
}

func (mudp *Mudp) handleMudpRequest(request cpMudp.MudpRequest) {
	mudp.mu.Lock()
	defer mudp.mu.Unlock()

	// validate requester
	envelope, err := request.Src()
	if err != nil {
		return
	}

	var rec peer.PeerRecord
	if _, err = record.ConsumeTypedEnvelope(envelope, &rec); err != nil {
		return
	}
	if rec.PeerID == mudp.h.ID() {
		return // request comes from itself
	}

	// check distance
	if err != nil {
		return
	}

	if dist([]byte(mudp.h.ID()), []byte(rec.PeerID))>>uint32(request.Distance()) != 0 {
		return
	}

	ns, err := request.Namespace()
	if err != nil {
		return
	}

	if _, ok := mudp.mustAdvertise[ns]; !ok {
		return
	}

	response, err := mudp.buildResponse(ns)
	if err == nil {
		go mudp.mc.Multicast(response)
	}
}

func (mudp *Mudp) buildResponse(ns string) ([]byte, error) {
	_, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		panic(err)
	}

	root, err := cpMudp.NewRootMudpPacket(seg)
	if err != nil {
		panic(err)
	}
	response, err := root.NewResponse()
	if err != nil {
		return nil, err
	}

	response.SetNamespace(ns)

	if cab, ok := peerstore.GetCertifiedAddrBook(mudp.h.Peerstore()); ok {
		env := cab.GetPeerRecord(mudp.h.ID())
		rec, err := env.Marshal()
		if err != nil {
			return nil, err
		}
		response.SetEnvelope(rec)
	}
	return root.Message().MarshalPacked()
}

func (mudp *Mudp) handleMudpResponse(response cpMudp.MudpResponse) {
	var (
		finder chan peer.AddrInfo
		rec    peer.PeerRecord
		err    error
		ok     bool
	)

	mudp.mu.Lock()
	defer mudp.mu.Unlock()

	ns, err := response.Namespace()
	if err != nil {
		return
	}

	if finder, ok = mudp.mustFind[ns]; !ok {
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

func (mudp *Mudp) Advertise(ctx context.Context, ns string, opt ...discovery.Option) (time.Duration, error) {
	mudp.mu.Lock()
	defer mudp.mu.Unlock()

	opts, err := mudp.options(ns, opt)
	if err != nil {
		return 0, err
	}

	if ttlChan, ok := mudp.mustAdvertise[ns]; ok {
		ttlChan <- opts.Ttl
	} else {
		resetTtl := make(chan time.Duration)
		mudp.mustAdvertise[ns] = resetTtl
		go mudp.trackAdvertise(ns, resetTtl, opts.Ttl)
	}
	return opts.Ttl, nil
}

func (mudp *Mudp) options(ns string, opt []discovery.Option) (opts *discovery.Options, err error) {
	opts = &discovery.Options{}
	if err = opts.Apply(opt...); err == nil && opts.Ttl == 0 {
		opts.Ttl = discTTL
	}

	return
}

func (mudp *Mudp) trackAdvertise(ns string, resetTtl chan time.Duration, ttl time.Duration) {
	timer := time.NewTimer(ttl)
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			mudp.mu.Lock()

			select { // check again TTL after acquiring lock
			case ttl := <-resetTtl:
				timer.Reset(ttl)
				mudp.mu.Unlock()
			default:
				close(resetTtl)
				delete(mudp.mustAdvertise, ns)
				mudp.mu.Unlock()
				return
			}
		case ttl := <-resetTtl:
			timer.Reset(ttl)
		}
	}
}

func (mudp *Mudp) FindPeers(ctx context.Context, ns string, opt ...discovery.Option) (<-chan peer.AddrInfo, error) {
	mudp.mu.Lock()
	defer mudp.mu.Unlock()

	var (
		opts *discovery.Options
		dist uint8
		err  error
	)

	opts, err = mudp.options(ns, opt)
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

	request, err := mudp.buildRequest(ns, dist)
	if err != nil {
		return nil, err
	}

	finder := make(chan peer.AddrInfo, opts.Limit)
	mudp.mustFind[ns] = finder

	go mudp.mc.Multicast(request)
	go mudp.closeFindPeers(ns, opts.Ttl)

	return finder, nil
}

func (mudp *Mudp) closeFindPeers(ns string, ttl time.Duration) {
	timer := time.NewTimer(ttl)
	defer timer.Stop()

	<-timer.C

	mudp.mu.Lock()
	defer mudp.mu.Unlock()

	if finder, ok := mudp.mustFind[ns]; ok {
		close(finder)
		delete(mudp.mustFind, ns)
	}
}

func (mudp *Mudp) buildRequest(ns string, dist uint8) ([]byte, error) {
	_, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		panic(err)
	}

	root, err := cpMudp.NewRootMudpPacket(seg)
	if err != nil {
		panic(err)
	}
	request, err := root.NewRequest()
	if err != nil {
		return nil, err
	}

	request.SetNamespace(ns)
	request.SetDistance(dist)
	if cab, ok := peerstore.GetCertifiedAddrBook(mudp.h.Peerstore()); ok {
		env := cab.GetPeerRecord(mudp.h.ID())
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
