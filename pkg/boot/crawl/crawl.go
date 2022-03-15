package crawl

import (
	"context"
	"io"
	"io/ioutil"
	"net"
	"time"

	"github.com/lthibault/log"

	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/record"

	netutil "github.com/wetware/casm/pkg/util/net"
)

type Dialer interface {
	DialContext(ctx context.Context, network, addr string) (net.Conn, error)
}

type DialStrategy interface {
	Dial(context.Context, Dialer) (<-chan net.Conn, error)
}

type Scanner interface {
	Scan(net.Conn, record.Record) (*record.Envelope, error)
}

type Crawler struct {
	Logger   log.Logger
	Dialer   Dialer
	Strategy DialStrategy
	Scanner  Scanner
}

func (c Crawler) FindPeers(ctx context.Context, ns string, opt ...discovery.Option) (<-chan peer.AddrInfo, error) {
	if c.Logger == nil {
		c.Logger = log.New(log.WithLevel(log.FatalLevel))
	}

	if c.Scanner == nil {
		c.Scanner = basicScanner{}
	}

	c.Logger = c.Logger.WithField("ns", ns)

	var opts discovery.Options
	if err := opts.Apply(opt...); err != nil {
		return nil, err
	}

	conns, err := c.Strategy.Dial(ctx, c.dialer())
	if err != nil {
		return nil, err
	}

	out := make(chan peer.AddrInfo, 8)
	go func() {
		defer close(out)

		c.Logger.Trace("crawl started")
		defer c.Logger.Tracef("crawl finished")

		var rec peer.PeerRecord
		for conn := range conns {
			if c.read(ctx, conn, &rec) {
				select {
				case out <- peer.AddrInfo{ID: rec.PeerID, Addrs: rec.Addrs}:
					if opts.Limit--; opts.Limit == 0 {
						return
					}

				case <-ctx.Done():
				}
			}
		}
	}()

	return out, nil
}

func (c Crawler) read(ctx context.Context, conn net.Conn, r record.Record) bool {
	defer conn.Close()

	if err := c.deadline(ctx, conn); err != nil {
		c.Logger.WithError(err).Debug("unable to set deadline")
		return false
	}

	_, err := c.scanner().Scan(conn, r)
	if err != nil {
		c.Logger.WithError(err).Debug("scan failed")
	}

	return err == nil
}

func (c Crawler) deadline(ctx context.Context, conn net.Conn) error {
	if t, ok := ctx.Deadline(); ok {
		return conn.SetDeadline(t)
	}

	return conn.SetDeadline(netutil.Time().Add(time.Second))
}

func (c Crawler) dialer() Dialer {
	if c.Dialer == nil {
		return new(net.Dialer)
	}

	return c.Dialer
}

func (c Crawler) scanner() Scanner {
	if c.Scanner == nil {
		return basicScanner{}
	}

	return c.Scanner
}

type basicScanner struct{}

func (basicScanner) Scan(conn net.Conn, dst record.Record) (*record.Envelope, error) {
	data, err := ioutil.ReadAll(io.LimitReader(conn, 4096)) // arbitrary MTU
	if err != nil {
		return nil, err
	}

	return record.ConsumeTypedEnvelope(data, dst)
}
