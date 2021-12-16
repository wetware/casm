package boot

import (
	"context"
	"sync"

	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/peer"
)

type DiscoveryService struct {
	Strategy ScanStrategy
	Net      Dialer

	once  sync.Once
	scans chan *scanRequest
}

func (d *DiscoveryService) FindPeers(ctx context.Context, ns string, opt ...discovery.Option) (<-chan peer.AddrInfo, error) {
	d.once.Do(func() { d.scans = make(chan *scanRequest) })

	opts := &discovery.Options{
		Limit: 1,
	}

	if err := opts.Apply(opt...); err != nil {
		return nil, err
	}

	chout := make(chan peer.AddrInfo)
	select {
	case d.scans <- &scanRequest{opts: opts, chout: chout}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return chout, nil
}

func (d *DiscoveryService) Serve(ctx context.Context) error {
	d.once.Do(func() { d.scans = make(chan *scanRequest) })

	var (
		errs = make(chan error, 1)
	)

	for {
		select {
		case scan := <-d.scans:
			scanner := scan.Bind(d.Net, d.Strategy)
			go d.run(ctx, func(e error) { errs <- e }, scanner)

		case err := <-errs:
			return err

		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (d *DiscoveryService) run(ctx context.Context, raise func(error), scan func(context.Context) error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	for {
		if err := scan(ctx); err == nil {
			break
		}
	}
}

type scanRequest struct {
	opts  *discovery.Options
	chout chan<- peer.AddrInfo
}

func (s scanRequest) Bind(d Dialer, strategy ScanStrategy) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		var rec peer.PeerRecord

		_, err := strategy.Scan(ctx, d, &rec)
		if err == nil {
			select {
			case s.chout <- peer.AddrInfo{ID: rec.PeerID, Addrs: rec.Addrs}:
			case <-ctx.Done():
				err = ctx.Err()
			}
		}

		return err
	}
}
