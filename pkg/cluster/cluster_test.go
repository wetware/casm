package cluster_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/stretchr/testify/require"
	"github.com/wetware/casm/pkg/cluster"
	mx "github.com/wetware/matrix/pkg"
	"github.com/wetware/matrix/pkg/netsim"
)

func TestCluster_init(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := mx.New(ctx).MustHost(ctx)
	p, err := pubsub.NewGossipSub(ctx, h)
	require.NoError(t, err)

	c, err := cluster.New(h, p)
	require.NoError(t, err)
	require.NotNil(t, c)

	require.Equal(t, "casm", c.String())
	require.NotNil(t, c.Process())

	require.NoError(t, c.Close())
}

func TestCluster_simulation(t *testing.T) {
	t.Parallel()

	const (
		n   = 8 // // FIXME:  test fails when this is increased
		lim = 4 // discovery limit
		ttl = time.Millisecond * 200
	)

	var (
		dopt = pubsub.WithDiscoveryOpts(discovery.Limit(lim))
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sim := mx.New(ctx)

	hs := sim.MustHostSet(ctx, n)
	ds := make([]discovery.Discovery, n)
	ps := make([]*pubsub.PubSub, n)
	cs := make([]*cluster.Cluster, n)

	mx.
		// set up
		Go(func(ctx context.Context, i int, h host.Host) error {
			ds[i] = sim.NewDiscovery(h, &netsim.SelectRing{})
			return nil
		}).
		Go(func(ctx context.Context, i int, h host.Host) (err error) {
			ps[i], err = pubsub.NewGossipSub(ctx, h,
				pubsub.WithDiscovery(ds[i], dopt))
			return
		}).
		Go(func(ctx context.Context, i int, h host.Host) (err error) {
			cs[i], err = cluster.New(h, ps[i],
				cluster.WithTTL(ttl))
			return
		}).
		// test simulation
		Go(func(ctx context.Context, i int, h host.Host) error {
			ticker := time.NewTicker(time.Millisecond * 100)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					if u := count(cs[i]); u != n {
						t.Logf("%s:  %d", h.ID().Pretty(), u)
						continue
					}

					// HACK:  idealy we would test iterators in their own test function,
					//        but its quite tedious set up a cluster, so we'll piggyback
					//		  here.
					//
					// Addendum:  it's not a hack if there's a comment =P
					t.Run(fmt.Sprintf("Iterator_entries_in_heap_order/%s", h.ID().ShortString()), func(t *testing.T) {
						var old, new time.Time
						for it := cs[i].Iter(); it.Next(); {
							require.Equal(t, cluster.RecordType_None, it.Record().Which())

							if new = it.Deadline(); old.IsZero() {
								require.True(t, it.More()) // dumb, but ensures 100% test coverage
							} else {
								require.False(t, new.Before(old),
									"heap violation (%v happens before %v)", new, old)
							}

							old = new
						}
					})

					return nil

				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}).
		// tear down
		Go(func(ctx context.Context, i int, h host.Host) error { return cs[i].Close() }).
		Go(func(ctx context.Context, i int, h host.Host) error { return h.Close() }).
		Must(ctx, hs)
}

func count(c *cluster.Cluster) int {
	var ps peer.IDSlice
	for it := c.Iter(); it.Next(); {
		ps = append(ps, it.Peer())
	}
	return len(ps)
}
