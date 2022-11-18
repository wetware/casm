package crawl

import (
	"crypto/rand"
	"fmt"
	"math/bits"
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
	Port     int
	Net      *net.IPNet
	ip, bcst net.IP
	mask     net.IPMask
}

// CIDR returns a range that iterates through a block of IP addreses
// in pseudorandom order, with a fixed port.
func NewCIDR(cidr string, port int) Strategy {
	return func() (Range, error) {
		_, subnet, err := net.ParseCIDR(cidr)
		c := &CIDR{
			Port: port,
			Net:  subnet,
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
	c.resetIP()
	c.setBroadcast()
	c.setRandomMask()
}

// Next updates addr with the next IP in the CIDR block traversal, and
// the target port.   The supplied Addr MUST be a non-nil *net.UDPAddr
// or *net.TCPAddr.  If addr.IP == nil, it is automatically populated.
// Note that a call to Next MAY use addr.IP as scratch space, even if
// the call to Next returns false.  When Next returns false, the CIDR
// iteration has been exhausted.
func (c *CIDR) Next(addr net.Addr) bool {
	switch a := addr.(type) {
	case *net.UDPAddr:
		c.bind((*addrKind)(a))

	case *net.TCPAddr:
		c.bind((*addrKind)(a))
	}

	return c.more()
}

func (c *CIDR) more() bool {
	return c.Net.Contains(c.ip)
}

func (c *CIDR) bind(addr *addrKind) {
	addr.Port = c.Port
	addr.IP = c.nextIP(addr.IP)
}

func (c *CIDR) ensureIP(ip net.IP) net.IP {
	if cap(ip) < len(c.Net.IP) {
		return make(net.IP, len(c.Net.IP), net.IPv6len)
	}

	return ip[:len(c.Net.IP)]
}

type addrKind struct {
	IP   net.IP
	Port int
	Zone string
}

func (c *CIDR) nextIP(ip net.IP) net.IP {
	// first call?
	if ip = c.ensureIP(ip); len(c.ip) == 0 {
		c.ip = c.ip[:len(ip)]
		copy(c.ip, c.Net.IP)
	} else {
		c.incrIP()
	}

	for c.skip(ip) {
		c.incrIP()
	}

	return ip
}

func (c *CIDR) skip(ip net.IP) bool {
	copy(ip, c.ip)
	xormask(ip, c.mask)
	return ip.Equal(c.bcst) || ip.Equal(c.Net.IP)
}

func (c *CIDR) incrIP() {
	for i := len(c.ip) - 1; i >= 0; i-- {
		if c.ip[i]++; c.ip[i] > 0 {
			break
		}
	}
}

func (c *CIDR) resetIP() {
	if c.ip == nil {
		c.ip = make(net.IP, net.IPv6len)
	}
	c.ip = c.ip[:0]
}

func (c *CIDR) setBroadcast() {
	if cap(c.bcst) < net.IPv6len {
		c.bcst = make(net.IP, net.IPv6len)
	}
	c.bcst = c.bcst[:len(c.Net.IP)]

	mask := c.Net.Mask
	copy(c.bcst, c.Net.IP)

	for i := 0; i < len(mask); i++ {
		ipIdx := len(c.bcst) - i - 1
		c.bcst[ipIdx] = c.Net.IP[ipIdx] | ^mask[len(mask)-i-1]
	}
}

func (c *CIDR) setRandomMask() {
	if cap(c.mask) < len(c.Net.IP) {
		c.mask = make(net.IPMask, 0, net.IPv6len)
	}
	c.mask = c.mask[:len(c.Net.IP)]

	// generate a random mask
	rand.Read(c.mask)

	// Compute the complement of the netmask.  This tells us which
	// bits of the address should actually be randomized.  For example,
	// only the last 8 bits of a /24 block should be randomized.
	complement(c.Net.Mask)
	defer complement(c.Net.Mask) // reset to the original when finished

	// Set the leading bits to match the network IP, and the trailing bits
	// to match the random mask. For example, in a /24 block, the first 24
	// bits will match the network IP, and the final 8 bits will match the
	// random mask.
	andmask(c.mask, c.Net.Mask)
}

func complement(m []byte) {
	for i := range m {
		m[i] ^= 0xFF
	}
}

func xormask(dst, src []byte) {
	for i := range dst {
		dst[i] ^= src[i]
	}
}

func andmask(dst, src []byte) {
	for i := range dst {
		dst[i] &= src[i]
	}
}
