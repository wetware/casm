package multicast

import (
	"capnproto.org/go/capnp/v3"
	"github.com/wetware/casm/internal/api/boot"
)

/*
 * util.go contains unexported utilities
 */

// // NewTransport parses a multiaddr and returns the corresponding transport,
// // if it exists.  Note that addresses MUST start with a protocol identifier
// // such as '/multicast'.
// func NewTransport(m ma.Multiaddr) (_ Transport, err error) {
// 	switch h := head(m); h.Protocol().Code {
// 	case P_MCAST:
// 		if m, err = ma.NewMultiaddrBytes(h.RawValue()); err != nil {
// 			return nil, err
// 		}

// 		return NewMulticastUDP(m)

// 	default:
// 		return nil, fmt.Errorf("invalid boot protocol '%s'", h.String())
// 	}
// }

// func head(m ma.Multiaddr) (c ma.Component) {
// 	ma.ForEach(m, func(cc ma.Component) bool {
// 		c = cc
// 		return false
// 	})
// 	return
// }

type multicastPacketPool chan *boot.MulticastPacket

var packetPool = make(multicastPacketPool, 16)

func (pool multicastPacketPool) GetQueryPacket(ns string) (*boot.MulticastPacket, error) {
	pkt, err := pool.Get()
	if err == nil {
		err = pkt.SetQuery(ns)
	}

	return pkt, err
}

func (pool multicastPacketPool) GetResponsePacket() (*boot.MulticastPacket, error) {
	return pool.Get()
}

func (pool multicastPacketPool) Get() (*boot.MulticastPacket, error) {
	select {
	case pkt := <-pool:
		return pkt, nil
	default:
	}

	_, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		return nil, err
	}

	pkt, err := boot.NewRootMulticastPacket(seg)
	return &pkt, err
}

func (pool multicastPacketPool) Put(pkt *boot.MulticastPacket) {
	select {
	case pool <- pkt:
	default:
	}
}

type errChanPool chan chan error

var chErrPool = make(errChanPool, 16)

func (pool errChanPool) Get() (cherr chan error) {
	select {
	case cherr = <-pool:
	default:
		cherr = make(chan error, 1)
	}

	return
}

func (pool errChanPool) Put(cherr chan error) {
	select {
	case <-cherr:
	default:
	}

	select {
	case pool <- cherr:
	default:
	}
}
