package boot

import (
	"context"
	"net"
	"runtime"
	"sync"

	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/record"
	"golang.org/x/sync/semaphore"
)

type Crawler struct {
	Net      Dialer
	Strategy ScanStrategy
}

func (c Crawler) FindPeers(ctx context.Context, ns string, opt ...discovery.Option) (<-chan peer.AddrInfo, error) {
	opts := &discovery.Options{
		Limit: 1,
	}

	if err := opts.Apply(opt...); err != nil {
		return nil, err
	}

	out := make(chan peer.AddrInfo)
	go func() {
		defer close(out)

		var rec peer.PeerRecord
		for c.nextPeer(ctx, &rec) {
			select {
			case out <- peer.AddrInfo{ID: rec.PeerID, Addrs: rec.Addrs}:
			case <-ctx.Done():
			}
		}
	}()

	return out, nil
}

func (c Crawler) nextPeer(ctx context.Context, r record.Record) bool {
	_, err := c.Strategy.Scan(ctx, c.Net, r)
	return err != nil
}

// LimitedDialer limits the number of concurrent scans.
type LimitedDialer struct {
	Dialer Dialer
	Limit  int

	once sync.Once
	s    *semaphore.Weighted
}

func (d *LimitedDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	d.once.Do(func() {
		if d.Dialer == nil {
			d.Dialer = new(net.Dialer)
		}

		if d.Limit == 0 {
			d.Limit = 8
		}

		d.s = semaphore.NewWeighted(int64(d.Limit))
	})

	if err := d.s.Acquire(ctx, 1); err != nil {
		return nil, err
	}

	conn, err := d.Dialer.DialContext(ctx, network, addr)
	if err != nil {
		d.s.Release(1)
		return nil, err
	}

	var (
		once sync.Once
		free = func() { once.Do(func() { d.s.Release(1) }) }
	)

	tc := &trackedConn{
		Conn: conn,
		free: free,
	}

	runtime.SetFinalizer(tc, func(tc *trackedConn) { free() })

	return tc, nil
}

type trackedConn struct {
	net.Conn
	free func()
}

func (tc *trackedConn) Close() (err error) {
	defer tc.free()
	return tc.Conn.Close()
}
