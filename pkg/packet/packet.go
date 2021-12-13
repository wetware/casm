package packet

import (
	"bytes"
	"errors"
	"fmt"
	"net"

	"github.com/libp2p/go-libp2p-core/record"
	"github.com/oxtoacart/bpool"
	casm "github.com/wetware/casm/pkg"
)

var _ RoundTripper = (*Endpoint)(nil)

const (
	// A datagram where len(payload) <= 508  should generally be safe
	// from fragmentation: https://stackoverflow.com/a/35697810/1156707.
	//
	// We reserve an additional 60 bytes for out-of-band data, resulting
	// in a maximum packet size of 448 bytes.
	MaxMsgSize = 448

	PacketEnvelopeDomain = "casm.packet"
)

type Handler interface {
	ServePacket(buf []byte, n int) (int, error)
}

type RoundTripper interface {
	RoundTrip(net.PacketConn, record.Record) (*casm.Register, error)
}

type Endpoint struct {
	Addr net.Addr
	*casm.Register
}

// ServePacket passes the contents of the register to the handler.
func (b Endpoint) ServePacket(buf []byte, n int) (int, error) {
	if e, ok := b.Register.Load(); ok {
		b, err := e.Marshal()
		if err == nil {
			n = copy(buf, b)
		}

		return n, err
	}

	return 0, errors.New("empty")
}

// RoundTrip sends the contents of the register to Addr, then
// returns any response in a new register.
func (b Endpoint) RoundTrip(conn net.PacketConn, r record.Record) (*casm.Register, error) {
	buf := pool.Get()
	defer pool.Put(buf)

	if e, ok := b.Load(); ok {
		data, err := e.Marshal()
		if err != nil {
			return nil, fmt.Errorf("marshal envelope: %w", err)
		}

		if len(data) > MaxMsgSize {
			return nil, errors.New("packet overflow")
		}

		if cap(data) >= MaxMsgSize {
			pool.Put(bytes.NewBuffer(data[0:]))
		}

		buf.Write(data)
	}

	if _, err := conn.WriteTo(buf.Bytes(), b.Addr); err != nil {
		return nil, fmt.Errorf("send to %s: %w", b.Addr, err)
	}

	data := buf.Bytes()[:MaxMsgSize]
	n, _, err := conn.ReadFrom(data)
	if err != nil {
		return nil, err
	}

	e, err := record.ConsumeTypedEnvelope(data[:n], r)
	return casm.New(e), err
}

var pool = bpool.NewSizedBufferPool(32, MaxMsgSize)
