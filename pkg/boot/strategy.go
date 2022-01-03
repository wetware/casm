package boot

import (
	"context"
	"encoding/binary"
	"fmt"
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/lthibault/log"
)

var _ DialStrategy = (*ScanSubnet)(nil)

// ScanSubnet is a brute-force dial strategy that exhaustively searches an
// entire Subnet block in pseudorandom order.
type ScanSubnet struct {
	Logger log.Logger
	Net    string
	Port   int
	CIDR   string

	once sync.Once
}

func (ss *ScanSubnet) Dial(ctx context.Context, d Dialer) (<-chan net.Conn, error) {
	ss.once.Do(func() {
		if ss.Logger == nil {
			ss.Logger = log.New()
		}

		if ss.Net == "" {
			ss.Net = "tcp"
		}

		if ss.CIDR == "" {
			ss.CIDR = "127.0.0.1"
		}

		if ss.Port == 0 {
			ss.Port = 8822
		}
	})

	iter, err := newSubnetIter(ss.CIDR)
	if err != nil {
		return nil, err
	}

	out := make(chan net.Conn, 1)
	go func() {
		defer close(out)

		ip := make(net.IP, 4)
		for ; iter.More(ctx); iter.Next() {
			if iter.Skip() {
				continue
			}

			iter.Scan(ip)

			conn, err := ss.dial(ctx, d, ip)
			if err == nil {
				select {
				case out <- conn:
				case <-ctx.Done():
				}
			}
		}
	}()

	return out, nil
}

func (ss *ScanSubnet) dial(ctx context.Context, d Dialer, ip net.IP) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Millisecond*10)
	defer cancel()

	ss.Logger.Tracef("dialing %s", ip)

	return d.DialContext(ctx,
		ss.Net,
		fmt.Sprintf("%v:%d", ip, ss.Port))
}

// iterates through a CIDR range in pseudorandom order.
type cidrIter struct {
	ip     net.IP
	subnet *net.IPNet

	mask, begin, end, i, rand uint32
}

func newSubnetIter(cidr string) (it *cidrIter, err error) {
	it = new(cidrIter)

	it.ip, it.subnet, err = net.ParseCIDR(cidr)

	// Convert IPNet struct mask and address to uint32.
	// Network is BigEndian.
	it.mask = binary.BigEndian.Uint32(it.subnet.Mask)
	it.begin = binary.BigEndian.Uint32(it.subnet.IP)
	it.end = (it.begin & it.mask) | (it.mask ^ 0xffffffff) // final address

	// Each IP will be masked with the nonce before knocking.
	// This effectively randomizes the search.
	it.rand = rand.Uint32() & (it.mask ^ 0xffffffff)

	it.i = it.begin
	return
}

func (c *cidrIter) More(ctx context.Context) bool {
	return ctx.Err() == nil && c.i <= c.end
}

func (c *cidrIter) Skip() bool {
	// Skip X.X.X.0 and X.X.X.255
	return c.i^c.rand == c.begin || c.i^c.rand == c.end
}

func (c *cidrIter) Next() {
	// Populate the current IP address.
	c.i++
	binary.BigEndian.PutUint32(c.ip, c.i^c.rand)
}

func (c *cidrIter) Scan(ip net.IP) {
	if len(ip) != 4 {
		panic(ip)
	}
	binary.BigEndian.PutUint32(ip, c.i^c.rand)
}
