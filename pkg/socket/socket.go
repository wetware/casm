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
// signed messages.  A socket's maximum transmission unit (MTU)
// is exactly 2KB.  Sockets will fail to unmarshal records from
// payloads exceeding 2 << 10 bytes. Bear in mind that payloads
// are automatically signed, which increases payload size.   In
// practice, an overhead of 128 bytes is generally sufficient.
type Socket struct {
	// Conn is a datagram-oriented connection that can read and
	// write packets.  This is most often a *net.UDPSocket, and
	// MAY be support multicast.
	Conn net.PacketConn

	// Sealer can marshal and sign a record.  Most applications
	// SHOULD use *CachingSealer.
	Sealer Sealer
}

// Close the underlying PacketConn.
func (s Socket) Close() error {
	return s.Conn.Close()
}

// Read a packet from the socket, unmarshal it, and verify its
// signature.  Returns the address that was in the packet, and
// the ID of the signing peer.
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

// Write the record to the specified address.
func (s Socket) Write(r record.Record, addr net.Addr) error {
	b, err := s.Sealer.Seal(r)
	if err == nil {
		_, err = s.Conn.WriteTo(b, addr)
	}

	return err
}
