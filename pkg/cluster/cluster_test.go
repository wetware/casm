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
	"github.com/wetware/casm/pkg/cluster/boot"
	mx "github.com/wetware/matrix/pkg"
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

	require.Equal(t, "casm", c.Topic().String())

	require.NoError(t, c.Close())
}

func TestCluster_simulation(t *testing.T) {
	t.Parallel()
	t.Skip("Skipping due to inexplicable failures (see FIXME)")

	const (
		n   = 8 // FIXME:  test fails when this is increased
		lim = 8 // FIXME:  test fails when n != lim
		ttl = time.Millisecond * 200
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sim := mx.New(ctx)

	hs := sim.MustHostSet(ctx, n)
	ds := make([]discovery.Discovery, n)
	ps := make([]*pubsub.PubSub, n)
	cs := make([]cluster.Model, n)

	mx.
		// set up
		Go(func(ctx context.Context, i int, h host.Host) error {
			as := make(boot.StaticAddrs, lim)
			for i, hh := range hs[:lim] {
				as[i] = *host.InfoFromHost(hh)
			}

			ds[i] = as
			return nil
		}).
		Go(func(ctx context.Context, i int, h host.Host) (err error) {
			ps[i], err = pubsub.NewFloodSub(ctx, h,
				pubsub.WithDiscovery(ds[i]))
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
		Must(ctx, hs)
}

func count(c cluster.Model) int {
	var ps peer.IDSlice
	for it := c.Iter(); it.Next(); {
		ps = append(ps, it.Peer())
	}
	return len(ps)
}
