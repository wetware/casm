package crawl

import (
	"encoding/binary"
	"math/bits"
	"math/rand"
	"net"
)

type Strategy func() Range

type Range interface {
	Next(a net.Addr) bool
}

type PortRange struct {
	IP    net.IP
	Mask  uint16
	pos   uint16
	shift int
}

// NewPortRange returns a range that iterates through ports on
// the specified IP.  The mask parameter is a bitmask used to
// select which ports should be scanned. PortRange is stateful,
// and ports are scanned in order.
//
// If len(ip) == 0, defaults to 127.0.0.1
// If the IP address is unspecified, defaults to the standard
// loopback address for the IP version.
//
// If mask == 0, defaults to match all non-reserved ports, i.e.
// all ports in the range (1024, 65535).
func NewPortRange(ip net.IP, mask uint16) *PortRange {
	pr := &PortRange{
		IP:   ip,
		Mask: mask,
	}

	pr.Reset()

	return pr
}

// Reset internal state, allowing p to be reused.  Does
// not affect IP or Mask.
func (p *PortRange) Reset() {
	switch {
	case p.IP.IsUnspecified():
		if p.IP.To4() == nil {
			p.IP = net.IPv6loopback
			break
		}

		fallthrough

	case len(p.IP) == 0:
		p.IP = net.IPv4(127, 0, 0, 1)
	}

	if p.Mask == 0 {
		p.Mask = 63 << 10 // matches all ports >1023
	}

	p.shift = bits.TrailingZeros16(p.Mask)
	p.pos = 0
}

func (p *PortRange) Next(addr net.Addr) (ok bool) {
	switch a := addr.(type) {
	case *net.UDPAddr:
		a.IP = p.IP
		a.Port, ok = p.nextPort()

	case *net.TCPAddr:
		a.IP = p.IP
		a.Port, ok = p.nextPort()
	}

	return
}

func (p *PortRange) nextPort() (int, bool) {
	for {
		p.pos++

		i := p.pos << p.shift
		if i > p.Mask {
			return 0, false // we're done
		}

		if i&p.Mask != 0 {
			return int(i), true
		}
	}
}

type CIDR struct {
	Port int
	ip   net.IP

	Subnet *net.IPNet

	mask, begin, end, i, rand uint32
}

// CIDR returns a range that iterates through a block of IP addreses
// in pseudorandom order, with a fixed port.
func NewCIDR(cidr string, port int) (*CIDR, error) {
	ip, subnet, err := net.ParseCIDR(cidr)

	c := &CIDR{
		ip:     ip,
		Port:   port,
		Subnet: subnet,
	}

	c.Reset()

	return c, err
}

// Reset internal state, allowing p to be reused.  Does
// not affect subnet or port.
func (c *CIDR) Reset() {
	// Convert IPNet struct mask and address to uint32.
	// Network is BigEndian.
	c.mask = binary.BigEndian.Uint32(c.Subnet.Mask)
	c.begin = binary.BigEndian.Uint32(c.Subnet.IP)
	c.end = (c.begin & c.mask) | (c.mask ^ 0xffffffff) // final address

	// Each IP will be masked with the nonce before knocking.
	// This effectively randomizes the search.
	c.rand = rand.Uint32() & (c.mask ^ 0xffffffff)

	c.i = c.begin
}

// Reset internal state, allowing p to be reused.  Does
// not affect IP or Mask.
func (c *CIDR) Next(addr net.Addr) (ok bool) {
	switch a := addr.(type) {
	case *net.UDPAddr:
		a.Port = c.Port
		ok = c.nextIP(a.IP)

	case *net.TCPAddr:
		a.Port = c.Port
		ok = c.nextIP(a.IP)
	}

	return
}

func (c *CIDR) nextIP(ip net.IP) bool {
	for ; c.more(); c.next() {
		if !c.skip() {
			c.setIP4(ip) // TODO:  IPv6 support
		}

	}

	return false
}

func (c *CIDR) more() bool {
	return c.i <= c.end
}

func (c *CIDR) next() {
	// Populate the current IP address.
	c.i++
	binary.BigEndian.PutUint32(c.ip, c.i^c.rand)
}

func (c *CIDR) skip() bool {
	// Skip X.X.X.0 and X.X.X.255
	return c.i^c.rand == c.begin || c.i^c.rand == c.end
}

func (c *CIDR) setIP4(ip net.IP) {
	binary.BigEndian.PutUint32(ip, c.i^c.rand)
}
