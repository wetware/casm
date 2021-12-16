package boot

import (
	"encoding/binary"
	"math/rand"
	"net"
)

// Subnet is a brute-force strategy that exhaustively searches an
// entire Subnet block in pseudorandom order.
type Subnet struct {
	CIDR string
	Err  error

	ip     net.IP
	subnet *net.IPNet

	mask, begin, end, i, rand uint32
}

func (c *Subnet) Reset() {
	c.ip, c.subnet, c.Err = net.ParseCIDR(c.CIDR)

	// Convert IPNet struct mask and address to uint32.
	// Network is BigEndian.
	c.mask = binary.BigEndian.Uint32(c.subnet.Mask)
	c.begin = binary.BigEndian.Uint32(c.subnet.IP)
	c.end = (c.begin & c.mask) | (c.mask ^ 0xffffffff) // final address

	// Each IP will be masked with the nonce before knocking.
	// This effectively randomizes the search.
	c.rand = rand.Uint32() & (c.mask ^ 0xffffffff)

	c.i = c.begin
}

func (c *Subnet) More() bool { return c.i <= c.end }

func (c *Subnet) Skip() bool {
	// Skip X.X.X.0 and X.X.X.255
	return c.i^c.rand == c.begin || c.i^c.rand == c.end
}

func (c *Subnet) Next() {
	// Populate the current IP address.
	c.i++
	binary.BigEndian.PutUint32(c.ip, c.i^c.rand)
}

func (c *Subnet) Scan(ip net.IP) {
	if len(ip) != 4 {
		panic(ip)
	}
	binary.BigEndian.PutUint32(ip, c.i^c.rand)
}
