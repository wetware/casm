package crawl

import (
	"encoding/binary"
	"fmt"
	"math/bits"
	"math/rand"
	"net"
	"strconv"

	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
)

type Strategy func() (Range, error)

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
func NewPortScan(ip net.IP, mask uint16) Strategy {
	return func() (Range, error) {
		pr := &PortRange{
			IP:   ip,
			Mask: mask,
		}

		pr.Reset()

		return pr, nil
	}

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
func NewCIDR(cidr string, port int) Strategy {
	return func() (Range, error) {
		ip, subnet, err := net.ParseCIDR(cidr)

		// len(ip) == 32; resize if ipv4
		if isIPv4(ip) {
			ip = ip.To4()
		}

		c := &CIDR{
			ip:     ip,
			Port:   port,
			Subnet: subnet,
		}

		c.Reset()

		return c, err
	}
}

func ParseCIDR(maddr ma.Multiaddr) (Strategy, error) {
	_, addr, err := manet.DialArgs(maddr)
	if err != nil {
		return nil, err
	}

	host, portstr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}

	port, err := strconv.Atoi(portstr)
	if err != nil {
		return nil, err
	}

	ones, err := maddr.ValueForProtocol(P_CIDR)
	if err != nil {
		return nil, err
	}

	cidr := fmt.Sprintf("%s/%s", host, ones)
	return NewCIDR(cidr, port), nil
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
// not affect IP or Mask.  The addr instance MUST NOT be
// nil.
func (c *CIDR) Next(addr net.Addr) (ok bool) {
	switch a := addr.(type) {
	case *net.UDPAddr:
		a.Port = c.Port
		a.IP, ok = c.nextIP(a.IP)

	case *net.TCPAddr:
		a.Port = c.Port
		a.IP, ok = c.nextIP(a.IP)
	}

	return
}

func (c *CIDR) nextIP(ip net.IP) (_ net.IP, ok bool) {
	if ip == nil {
		ip = make(net.IP, len(c.ip))
	}

	for c.i <= c.end && !ok {
		if ok = !c.skip(); ok {
			c.setIP(ip) // TODO:  IPv6 support
		}

		c.i++
	}

	return ip, ok
}

func (c *CIDR) skip() bool {
	// Skip X.X.X.0 and X.X.X.255
	return c.i^c.rand == c.begin || c.i^c.rand == c.end
}

func (c *CIDR) setIP(ip net.IP) {
	// TODO:  IPv6 support
	binary.BigEndian.PutUint32(ip, c.i^c.rand)
}

func isIPv4(ip net.IP) bool {
	return ip.To4() != nil
}
