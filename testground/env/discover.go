package env

import (
	"context"
	"math/rand"

	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/lthibault/log"
	"github.com/testground/sdk-go/runtime"
	"github.com/testground/sdk-go/sync"
)

var t = sync.NewTopic("casm.mesh.announce", peer.AddrInfo{})

type DiscoveryClient struct {
	c   sync.Client
	r   *rand.Rand
	i   int // test-instance count
	log log.Logger
}

func (d DiscoveryClient) FindPeers(ctx context.Context, ns string, opt ...discovery.Option) (<-chan peer.AddrInfo, error) {
	d.log.Debug("discovery started")
	defer d.log.Debug("discovery finished")

	var (
		err     error
		options discovery.Options
		sub     *sync.Subscription
		ch      = make(chan peer.AddrInfo, d.i)
	)

	if err = options.Apply(opt...); err != nil {
		return nil, err
	}

	if sub, err = d.c.Subscribe(ctx, t, ch); err != nil {
		return nil, err
	}

	addrs := make([]peer.AddrInfo, d.i)
	for i := range addrs {
		select {
		case info := <-ch:
			addrs[i] = info
		case err = <-sub.Done():
			return nil, err
		}
	}

	d.r.Shuffle(len(addrs), func(i, j int) {
		addrs[i], addrs[j] = addrs[j], addrs[i]
	})

	ch = make(chan peer.AddrInfo, options.Limit)
	for _, info := range addrs[0:options.Limit] {
		ch <- info
	}
	close(ch)

	return ch, nil
}

func WithLimit(runEnv *runtime.RunEnv) discovery.Option {
	return func(opts *discovery.Options) error {
		if opts.Limit = runEnv.TestInstanceCount; opts.Limit > 3 {
			opts.Limit = 3
		}

		return nil
	}
}
