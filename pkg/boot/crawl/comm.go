package crawl

import (
	"net"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/record"
)

type comm struct {
	conn net.PacketConn
	done chan struct{}
}

type request struct {
	addr net.Addr
	ns   string
}

func newComm(conn net.PacketConn) comm {
	return comm{conn: conn, done: make(chan struct{})}
}

func (c comm) sendTo(data []byte, addr net.Addr) error {
	_, err := c.conn.WriteTo(data, addr)
	return err
}

func (c comm) sendToMultiple(s Strategy, data []byte) error {
	for ; s.More(); s.Next() {
		if s.Skip() {
			continue
		}

		if _, err := c.conn.WriteTo(data, s.Addr()); err != nil {
			if e, ok := err.(net.Error); ok && !e.Temporary() {
				return e
			}
			continue
		}
	}

	return nil
}

func (c comm) receiveRequests(resquests chan request) error {
	var buf [maxDatagramSize]byte

	for {
		n, addr, err := c.conn.ReadFrom(buf[:])
		if err != nil {
			if e, ok := err.(net.Error); ok && !e.Temporary() {
				return e
			}
			continue
		}

		data := make([]byte, n)
		copy(data, buf[:n])

		select {
		case resquests <- request{addr: addr, ns: string(data)}:
		case <-c.done:
			return nil
		}
	}
}

func (c comm) receiveResponses(responses chan peer.AddrInfo) error {
	var buf [maxDatagramSize]byte

	for {
		n, _, err := c.conn.ReadFrom(buf[:])
		if err != nil {
			if e, ok := err.(net.Error); ok && !e.Temporary() {
				return e
			}
			continue
		}

		var rec peer.PeerRecord
		if _, err = record.ConsumeTypedEnvelope(buf[:n], &rec); err == nil {
			select {
			case responses <- peer.AddrInfo{ID: rec.PeerID, Addrs: rec.Addrs}:
			case <-c.done:
				return nil
			}
		}
	}
}

func (c comm) close() {
	c.conn.Close()
	close(c.done)
}
