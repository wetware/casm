package crawl

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/record"
	netutil "github.com/wetware/casm/pkg/util/net"
)

const (
	maxDatagramSize = 8192
	defaultTTL      = time.Minute
	defaultTimeout  = time.Second
)

var ErrClosed = errors.New("closed")

type advRequest struct {
	ns  string
	ttl time.Duration
}

type Crawler struct {
	scanner Strategy

	done  <-chan struct{}
	cherr chan<- error
	err   atomic.Value

	t              time.Time
	advertisements chan<- advRequest
	mustAdvertise  map[string]time.Time

	rec atomic.Value

	transport Transport

	host host.Host
	once sync.Once
}

func New(h host.Host, addr net.Addr, scanner Strategy, opt ...Option) (*Crawler, error) {
	var (
		cherr          = make(chan error, 1)
		done           = make(chan struct{})
		advertisements = make(chan advRequest)
		requests       = make(chan request)
	)

	c := &Crawler{
		scanner:        scanner,
		done:           done,
		t:              netutil.Time(),
		advertisements: advertisements,
		mustAdvertise:  make(map[string]time.Time),
		cherr:          cherr,
		host:           h,
	}

	for _, option := range withDefaults(opt) {
		option(c)
	}

	conn, err := c.transport.Listen(addr)
	if err != nil {
		return nil, err
	}
	comm := newCommFromConn(conn)

	go func() {
		defer close(done)
		defer comm.close()

		go comm.receiveRequests(requests)

		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case t := <-ticker.C:
				for ns, deadline := range c.mustAdvertise {
					if t.After(deadline) {
						delete(c.mustAdvertise, ns)
					}
				}
			case adv := <-advertisements:
				c.mustAdvertise[adv.ns] = c.t.Add(adv.ttl)
			case r := <-requests:
				if _, ok := c.mustAdvertise[r.ns]; ok {
					envelope, _ := c.rec.Load().(*record.Envelope).Marshal()
					go comm.sendTo(envelope, r.addr)
				}
			case err := <-cherr:
				c.err.CompareAndSwap(error(nil), err)
				return
			}
		}
	}()

	return c, nil
}

func (c *Crawler) Advertise(ctx context.Context, ns string, opt ...discovery.Option) (time.Duration, error) {
	c.once.Do(func() {
		c.trackHostAddr(c.host)
	})

	var opts = discovery.Options{Ttl: defaultTTL}
	if err := opts.Apply(opt...); err != nil {
		return 0, err
	}

	select {
	case c.advertisements <- advRequest{ns: ns, ttl: opts.Ttl}:
		return opts.Ttl, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-c.done:
		return 0, ErrClosed
	}
}

func (c *Crawler) trackHostAddr(h host.Host) error {
	sub, err := h.EventBus().Subscribe(new(event.EvtLocalAddressesUpdated))
	if err != nil {
		return err
	}

	// Ensure the sync operation is run before anything else
	v, ok := <-sub.Out()
	if !ok {
		return fmt.Errorf("host %w", ErrClosed)
	}
	c.rec.Store(v.(event.EvtLocalAddressesUpdated).SignedPeerRecord)

	go func() {
		for v := range sub.Out() {
			c.rec.Store(v.(event.EvtLocalAddressesUpdated).SignedPeerRecord)
		}
	}()
	return nil
}

func (c *Crawler) FindPeers(ctx context.Context, ns string, opt ...discovery.Option) (<-chan peer.AddrInfo, error) {
	var (
		out = make(chan peer.AddrInfo, 8)
	)

	conn, err := c.transport.Dial(nil)
	if err != nil {
		return nil, err
	}
	comm := newCommFromConn(conn)

	ctx, cancel := context.WithCancel(ctx)

	go func() { // send requests
		comm.sendToMultiple(c.scanner, []byte(ns))
		time.Sleep(defaultTimeout)
		cancel()
	}()
	go comm.receiveResponses(out) // receive responses
	go func() {                   // stop finder
		select {
		case <-ctx.Done():
		case <-c.done:
		}

		comm.close()
		close(out)
	}()

	return out, nil
}

func (c *Crawler) Close() {
	select {
	case c.cherr <- ErrClosed:
	default:
	}

	<-c.done
}

type Strategy interface {
	More() bool
	Skip() bool
	Next()
	Addr() net.Addr
}

// iterates through a CIDR range in pseudorandom order.
type cidrIter struct {
	ip   net.IP
	port int

	subnet *net.IPNet

	mask, begin, end, i, rand uint32
}

func NewCIDR(cidr string, port int) (it *cidrIter, err error) {
	it = new(cidrIter)

	it.ip, it.subnet, err = net.ParseCIDR(cidr)
	it.port = port
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

func (c *cidrIter) More() bool {
	return c.i <= c.end
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

func (c *cidrIter) Addr() net.Addr {
	ip := make(net.IP, 4)

	binary.BigEndian.PutUint32(ip, c.i^c.rand)

	return &net.UDPAddr{IP: ip, Port: c.port}
}
