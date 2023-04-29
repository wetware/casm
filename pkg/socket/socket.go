// Package socket implements signed datagram sockets.
package socket

import (
	"net"

	"capnproto.org/go/capnp/v3/exp/bufferpool"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/record"
)

const maxDatagramSize = 2 << 10 // KB

// Socket is a packet-oriented network interface that exchanges
// signed messages.
type Socket struct {
	Conn   net.PacketConn
	Sealer Sealer
}

func (s Socket) Close() error {
	return s.Conn.Close()
}

func (s Socket) Read(r record.Record) (net.Addr, peer.ID, error) {
	buf := bufferpool.Default.Get(maxDatagramSize)

	n, addr, err := s.Conn.ReadFrom(buf)
	if err != nil {
		bufferpool.Default.Put(buf)
		return nil, "", err
	}

	e, err := record.ConsumeTypedEnvelope(buf[:n], r)
	if err != nil {
		bufferpool.Default.Put(buf)
		return nil, "", err
	}

	id, err := peer.IDFromPublicKey(e.PublicKey)
	if err != nil {
		bufferpool.Default.Put(buf)
		return nil, "", err
	}

	return addr, id, nil
}

func (s Socket) Write(r record.Record, addr net.Addr) error {
	b, err := s.Sealer.Seal(r)
	if err == nil {
		_, err = s.Conn.WriteTo(b, addr)
	}

	return err
}
