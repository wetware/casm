package pex

import (
	"context"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/discovery"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/lthibault/log"
)

/*
 * discovery.go contains a background discovery service based on the
 * implementation found in go-libp2p-pubsub, which bootstraps namespace
 * connectivity.
 */

type discover struct {
	d   discovery.Discovery
	opt []discovery.Option

	mu          sync.Mutex
	advertising map[string]context.CancelFunc
}

func newDiscover() *discover {
	return &discover{
		advertising: make(map[string]context.CancelFunc),
	}
}

func (disc *discover) StopTracking(ns string) {
	disc.mu.Lock()
	defer disc.mu.Unlock()

	if cancel, ok := disc.advertising[ns]; ok {
		cancel()
	}
}

// Bootstrap returns peers from the bootstrap discovery service.
func (disc *discover) Bootstrap(ctx context.Context, log log.Logger, ns string) (<-chan peer.AddrInfo, error) {
	disc.mu.Lock()
	defer disc.mu.Unlock()

	if _, ok := disc.advertising[ns]; !ok {
		disc.startTracking(ctx, log, ns)
	}

	return disc.d.FindPeers(ctx, ns, disc.opt...)
}

// caller must be holding c.mu
func (disc *discover) startTracking(ctx context.Context, log log.Logger, ns string) {
	ctx, disc.advertising[ns] = context.WithCancel(ctx)
	go func() {
		defer func() {
			disc.mu.Lock()
			defer disc.mu.Unlock()

			delete(disc.advertising, ns)
		}()

		ttl := disc.advertise(ctx, log, ns)

		t := time.NewTimer(ttl)
		defer t.Stop()

		for ctx.Err() == nil {
			select {
			case <-t.C:
				t.Reset(disc.advertise(ctx, log, ns))

			case <-ctx.Done():
				return
			}
		}
	}()
}

func (disc *discover) advertise(ctx context.Context, log log.Logger, ns string) time.Duration {
	ttl, err := disc.d.Advertise(ctx, ns)
	if err != nil {
		log.WithError(err).Warnf("error advertising %s", ns)
	}

	return interval(ttl)
}

func interval(ttl time.Duration) time.Duration {
	if ttl == 0 {
		return peerstore.TempAddrTTL
	}

	return ttl.Truncate(time.Millisecond * 100)
}
