package crawl

import (
	"context"
	"errors"
	"net"
	"strconv"
	"time"

	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"

	"github.com/lthibault/log"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/wetware/casm/pkg/boot/socket"
)

const P_CIDR = 103

func init() {
	if err := ma.AddProtocol(ma.Protocol{
		Name:       "cidr",
		Code:       P_CIDR,
		VCode:      ma.CodeToVarint(P_CIDR),
		Size:       8, // bits
		Transcoder: TranscoderCIDR{},
	}); err != nil {
		panic(err)
	}
}

var (
	ErrClosed = errors.New("closed")

	// ErrCIDROverflow is returned when a CIDR block is too large.
	ErrCIDROverflow = errors.New("CIDR overflow")
)

type Crawler struct {
	log    log.Logger
	lim    *socket.RateLimiter
	sock   *socket.Socket
	host   host.Host
	iter   Strategy
	cache  *socket.RecordCache
	done   <-chan struct{}
	cancel context.CancelFunc
}

func New(h host.Host, conn net.PacketConn, opt ...Option) *Crawler {
	ctx, cancel := context.WithCancel(context.Background())

	c := &Crawler{
		host:   h,
		done:   ctx.Done(),
		cancel: cancel,
	}

	for _, option := range withDefaults(opt) {
		option(c)
	}

	c.sock = socket.New(conn, c.lim, socket.Protocol{
		HandleError:   c.socketErrHandler(ctx),
		HandleRequest: c.requestHandler(ctx),
		Validate:      socket.BasicValidator(h.ID()),
		Cache:         c.cache,
	})

	return c
}

func (c *Crawler) Close() error {
	c.cancel()
	return c.sock.Close()
}

func running(ctx context.Context) func() bool {
	return func() bool {
		return ctx.Err() == nil
	}
}

func (c *Crawler) socketErrHandler(ctx context.Context) func(err error) {
	return func(err error) {
		if ctx.Err() == nil {
			c.log.WithError(err).Debug("socket error")
		}
	}
}

func (c *Crawler) requestHandler(ctx context.Context) func(socket.Request, net.Addr) {
	return func(r socket.Request, addr net.Addr) {
		ns, err := r.Namespace()
		if err != nil {
			c.log.WithError(err).Debug("error reading request namespace")
			return
		}

		if c.sock.Tracking(ns) {
			e, err := c.cache.LoadResponse(c.privkey(), ns)
			if err != nil {
				c.log.WithError(err).Error("error loading response from cache")
				return
			}

			if err = c.sock.Send(ctx, e, addr); err != nil {
				c.log.WithError(err).Debug("error sending response")
			}
		}
	}
}

func (c *Crawler) Advertise(ctx context.Context, ns string, opt ...discovery.Option) (time.Duration, error) {
	var opts = discovery.Options{Ttl: peerstore.TempAddrTTL}
	if err := opts.Apply(opt...); err != nil {
		return 0, err
	}

	return opts.Ttl, c.sock.Track(ctx, c.host, ns, opts.Ttl)
}

func (c *Crawler) FindPeers(ctx context.Context, ns string, opt ...discovery.Option) (<-chan peer.AddrInfo, error) {
	opts := discovery.Options{Limit: 1}
	if err := opts.Apply(opt...); err != nil {
		return nil, err
	}

	iter, err := c.iter()
	if err != nil {
		return nil, err
	}

	e, err := c.cache.LoadRequest(c.privkey(), ns)
	if err != nil {
		return nil, err
	}

	out, cancel := c.sock.Subscribe(ns, opts.Limit)
	go func() {
		defer cancel()

		var addr net.UDPAddr
		for c.active(ctx) && iter.Next(&addr) {
			if err := c.sock.Send(ctx, e, &addr); err != nil {
				c.log.
					WithError(err).
					WithField("to", &addr).
					Debug("failed to send request packet")
				return
			}
		}

		// Wait for response
		select {
		case <-ctx.Done():
		case <-c.done:
		}
	}()

	return out, nil
}

func (c *Crawler) active(ctx context.Context) (ok bool) {
	select {
	case <-ctx.Done():
	case <-c.done:
	default:
		ok = true
	}

	return
}

func (c *Crawler) privkey() crypto.PrivKey {
	return c.host.Peerstore().PrivKey(c.host.ID())
}

// TranscoderCIDR decodes a uint8 CIDR block
type TranscoderCIDR struct{}

func (ct TranscoderCIDR) StringToBytes(cidrBlock string) ([]byte, error) {
	num, err := strconv.ParseUint(cidrBlock, 10, 8)
	if err != nil {
		return nil, err
	}

	if num > 128 {
		return nil, ErrCIDROverflow
	}

	return []byte{uint8(num)}, err
}

func (ct TranscoderCIDR) BytesToString(b []byte) (string, error) {
	if len(b) > 1 || b[0] > 128 {
		return "", ErrCIDROverflow
	}

	return strconv.FormatUint(uint64(b[0]), 10), nil
}

func (ct TranscoderCIDR) ValidateBytes(b []byte) error {
	if uint8(b[0]) > 128 { // 128 is maximum CIDR block for IPv6
		return ErrCIDROverflow
	}

	return nil
}
