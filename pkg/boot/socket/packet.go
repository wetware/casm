package socket

import (
	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/wetware/casm/internal/api/boot"
)

type Request struct{ Record }

func (r Request) IsGradual() bool {
	return r.asPacket().Which() == boot.Packet_Which_gradualRequest
}

func (r Request) Distance() (dist uint8) {
	if r.IsGradual() {
		dist = r.asPacket().GradualRequest().Distance()
	}

	return
}

func (r Request) From() (id peer.ID, err error) {
	var s string
	if r.IsGradual() {
		s, err = r.asPacket().GradualRequest().From()
	} else {
		s, err = r.asPacket().Request()
	}

	if err == nil {
		id, err = peer.IDFromString(s)
	}

	return
}

type Response struct{ Record }

func (r Response) Peer() (id peer.ID, err error) {
	var s string
	if s, err = r.asPacket().Response().Peer(); err == nil {
		id, err = peer.IDFromString(s)
	}

	return
}

func (r Response) Addrs() ([]ma.Multiaddr, error) {
	addrs, err := r.asPacket().Response().Addrs()
	if err != nil {
		return nil, err
	}

	var (
		b  []byte
		as = make([]ma.Multiaddr, addrs.Len())
	)

	for i := range as {
		if b, err = addrs.At(i); err != nil {
			break
		}

		if as[i], err = ma.NewMultiaddrBytes(b); err != nil {
			break
		}
	}

	return as, err
}

func (r Response) Bind(info *peer.AddrInfo) (err error) {
	if info.ID, err = r.Peer(); err == nil {
		info.Addrs, err = r.Addrs()
	}

	return
}
