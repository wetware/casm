package crawl

import (
	"net"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/record"
)

type comm struct {
	conn *net.UDPConn
	done chan struct{}
}

type request struct {
	addr *net.UDPAddr
	ns   string
}

func newComm() (comm, error) {
	conn, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return comm{}, err
	}

	return comm{conn: conn, done: make(chan struct{})}, nil
}

func newCommFromCIDR(cidr string, port int) (comm, error) {
	ip, _, err := net.ParseCIDR(cidr)
	if err != nil {
		return comm{}, err
	}

	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: ip, Port: int(port)})
	if err != nil {
		return comm{}, err
	}
	return comm{conn: conn, done: make(chan struct{})}, nil
}

func (c comm) sendTo(data []byte, addr *net.UDPAddr) error {
	_, err := c.conn.WriteToUDP(data, addr)
	return err
}

func (c comm) sendToCIDR(cidr string, port int, data []byte) error {
	iter, err := newSubnetIter(cidr)
	if err != nil {
		return err
	}

	ip := make(net.IP, 4)
	for ; iter.More(); iter.Next() {
		if iter.Skip() {
			continue
		}

		iter.Scan(ip)

		if _, err := c.conn.WriteTo(data, &net.UDPAddr{IP: ip, Port: port}); err != nil {
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
		n, addr, err := c.conn.ReadFromUDP(buf[:])
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
