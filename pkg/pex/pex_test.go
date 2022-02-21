package pex_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/require"
	"github.com/wetware/casm/pkg/boot"
	"github.com/wetware/casm/pkg/pex"
	mx "github.com/wetware/matrix/pkg"
)

func TestPeX_Init(t *testing.T) {
	t.Parallel()
	t.Helper()

	ns := "init"

	t.Run("Fail to find unadvertised ns", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		h := mx.New(ctx).MustHost(ctx)

		px, err := pex.New(ctx, h)
		require.NoError(t, err)

		peers, err := px.FindPeers(ctx, ns)
		require.Error(t, err)
		require.Nil(t, peers)
	})

	t.Run("Fail_no_addrs", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		h := mx.New(ctx).MustHost(ctx, libp2p.NoListenAddrs)

		_, err := pex.New(ctx, h)
		require.EqualError(t, err, "host not accepting connections")
	})
}

func TestPeX_Bootstrap(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sim := mx.New(ctx)
	hs := sim.MustHostSet(ctx, 2)
	ps := make([]*pex.PeerExchange, len(hs))
	is := make([]peer.AddrInfo, len(hs))

	ns := "bootstrap"

	defer mx.Go(func(ctx context.Context, i int, h host.Host) error {
		ps[i].Close()
		return hs[i].Close()
	}).Must(ctx, hs)

	mx.
		Go(func(ctx context.Context, i int, h host.Host) (err error) {
			is[i] = *host.InfoFromHost(h)
			ps[i], err = pex.New(ctx, h)
			return
		}).
		Go(func(ctx context.Context, i int, h host.Host) error {
			if i == 0 {
				is[1] = *host.InfoFromHost(h)
			} else {
				is[0] = *host.InfoFromHost(h)
			}
			return nil
		}).
		Go(func(ctx context.Context, i int, h host.Host) error {
			if i != 0 {
				_, err := ps[i].Advertise(ctx, ns)
				require.NoError(t, err)
			}
			return nil
		}).
		Go(func(ctx context.Context, i int, h host.Host) error {
			joinCtx, joinCtxCancel := context.WithTimeout(ctx, time.Second)
			defer joinCtxCancel()

			if i == 0 {
				return ps[i].Bootstrap(joinCtx, ns, is[i])
			}

			return nil
		}).
		Go(func(ctx context.Context, i int, h host.Host) error {
			// TODO:  proper synchronization to ensure the 'Join()' call has completed
			time.Sleep(time.Millisecond)

			ch, err := ps[i].FindPeers(ctx, ns)
			require.NoError(t, err)

			info, ok := <-ch
			require.True(t, ok)
			require.Equal(t, is[i].ID, info.ID)
			return nil
		}).Must(ctx, hs)
}

func TestPeX_Advertise(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hs := mx.New(ctx).MustHostSet(ctx, 8)
	ps := make([]*pex.PeerExchange, len(hs))
	as := make(boot.StaticAddrs, len(hs))

	ns := "advertise"

	defer mx.Go(func(ctx context.Context, i int, h host.Host) error {
		ps[i].Close()
		return hs[i].Close()
	}).Must(ctx, hs)

	mx.
		Go(func(ctx context.Context, i int, h host.Host) (err error) {
			as[i] = *host.InfoFromHost(h)
			return
		}).
		Go(func(ctx context.Context, i int, h host.Host) (err error) {
			b := make(boot.StaticAddrs, 0, len(as)-1)
			for _, info := range as {
				if info.ID != h.ID() {
					b = append(b, info)
				}
			}

			ps[i], err = pex.New(ctx, h, pex.WithDiscovery(b))
			return
		}).Must(ctx, hs)

	_, err := ps[0].Advertise(ctx, ns)
	require.NoError(t, err)
}

func TestPeX_SingleNode(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ns := "single-node"

	h := mx.New(ctx).MustHost(ctx)

	px, err := pex.New(ctx, h)
	require.NoError(t, err, "should construct PeerExchange")
	require.NotNil(t, px, "should return PeerExchange")

	ttl, err := px.Advertise(ctx, ns)
	require.NoError(t, err)
	require.LessOrEqual(t, ttl, 5*time.Minute,
		"should return TTL less or equal to default Tick of 5 minutes")
	require.GreaterOrEqual(t, ttl, 5*time.Minute/2,
		"should return TTL greater or equal to half of the default Tick of 5 minutes")

	finder, err := px.FindPeers(ctx, ns)
	require.NoError(t, err)

	_, ok := <-finder
	require.False(t, ok, "shouldn't find any peer, the channel should close directly")
}

func TestPeX_TwoNodes(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		sim = mx.New(ctx)
		hs  = sim.MustHostSet(ctx, 2)
		ps  = make([]*pex.PeerExchange, len(hs))
	)

	ns := "two-nodes"

	defer mx.Go(func(ctx context.Context, i int, h host.Host) error {
		ps[i].Close()
		return hs[i].Close()
	}).Must(ctx, hs)

	mx.
		Go(func(ctx context.Context, i int, h host.Host) (err error) {
			if i == 0 {
				ps[i], err = pex.New(ctx, h)
			} else {
				ps[i], err = pex.New(ctx, h, pex.WithBootstrapPeers(*host.InfoFromHost(hs[0])))
			}
			require.NoError(t, err, "should construct PeerExchange")
			require.NotNil(t, ps[i], "should return PeerExchange")
			return err
		}).
		Go(func(ctx context.Context, i int, h host.Host) error {
			ttl, err := ps[i].Advertise(ctx, ns)
			require.NoError(t, err)
			require.LessOrEqual(t, ttl, 5*time.Minute,
				"should return TTL less or equal to default Tick of 5 minutes")
			require.GreaterOrEqual(t, ttl, 5*time.Minute/2,
				"should return TTL greater or equal to half of the default Tick of 5 minutes")
			return nil
		}).
		Go(func(ctx context.Context, i int, h host.Host) error {
			if i == 1 {
				infos, err := peers(ctx, ps[i], ns)
				require.NoError(t, err)
				require.Len(t, infos, 2)
				require.Equal(t, infos[0].ID, hs[0].ID())
			}
			return nil
		}).Must(ctx, hs)
}

const N = 10

func TestPex_NNodes(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ns := "n-nodes"

	var (
		newGossip = func(ns string) pex.GossipConfig {
			config := pex.DefaultGossipConfig
			config.Tick = time.Millisecond
			return config
		}

		sim = mx.New(ctx)
		hs  = sim.MustHostSet(ctx, N)
		ps  = make([]*pex.PeerExchange, len(hs))
	)

	defer mx.Go(func(ctx context.Context, i int, h host.Host) error {
		ps[i].Close()
		return hs[i].Close()
	}).Must(ctx, hs)

	err := mx.
		Go(func(ctx context.Context, i int, h host.Host) (err error) {
			ps[i], err = pex.New(ctx, h, pex.WithGossip(newGossip))
			if err != nil {
				return err
			}
			_, err = ps[i].Advertise(ctx, ns)
			return err
		}).
		Go(func(ctx context.Context, i int, h host.Host) error {
			if i != 0 {
				err := ps[i].Bootstrap(ctx, ns, *host.InfoFromHost(hs[i-1]))
				if err != nil {
					return err
				}
			}

			go func() {
				var (
					next = time.Duration(0)
				)
				for {
					select {
					case <-time.After(next):
						next, _ = ps[i].Advertise(ctx, ns)
					case <-ctx.Done():
						return
					}
				}
			}()

			return nil
		}).
		Err(ctx, hs)

	require.NoError(t, err)

	require.Eventually(t, func() bool {
		infos, err := peers(ctx, ps[0], ns)
		require.NoError(t, err)
		return len(infos) == min(pex.DefaultMaxView, N-1)
	}, 10*time.Second, 10*time.Millisecond)
}

func min(n1, n2 int) int {
	if n1 < n2 {
		return n1
	}
	return n2
}

func peers(ctx context.Context, px *pex.PeerExchange, ns string) ([]peer.AddrInfo, error) {
	finder, err := px.FindPeers(ctx, ns)
	if err != nil {
		return nil, err
	}

	infos := make([]peer.AddrInfo, 0)
	for info := range finder {
		infos = append(infos, info)
	}
	return infos, nil
}

func TestPeX_DisconnectedNode(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		newGossip = func(ns string) pex.GossipConfig {
			config := pex.DefaultGossipConfig
			config.Timeout = 5 * time.Second
			return config
		}

		sim = mx.New(ctx)
		hs  = sim.MustHostSet(ctx, 2)
		ps  = make([]*pex.PeerExchange, len(hs))
	)

	ns := "disconnected-node"

	defer mx.Go(func(ctx context.Context, i int, h host.Host) error {
		ps[i].Close()
		return hs[i].Close()
	}).Must(ctx, hs)

	mx.
		Go(func(ctx context.Context, i int, h host.Host) (err error) {
			if i == 0 {
				ps[i], err = pex.New(ctx, h)
			} else {
				ps[i], err = pex.New(ctx, h,
					pex.WithBootstrapPeers(*host.InfoFromHost(hs[0])),
					pex.WithGossip(newGossip),
				)
			}
			require.NoError(t, err, "should construct PeerExchange")
			require.NotNil(t, ps[i], "should return PeerExchange")
			return err
		}).
		Go(func(ctx context.Context, i int, h host.Host) error {
			ttl, err := ps[i].Advertise(ctx, ns)
			require.NoError(t, err)
			require.LessOrEqual(t, ttl, 5*time.Minute,
				"should return TTL less or equal to default Tick of 5 minutes")
			require.GreaterOrEqual(t, ttl, 5*time.Minute/2,
				"should return TTL greater or equal to half of the default Tick of 5 minutes")
			return nil
		}).
		Go(func(ctx context.Context, i int, h host.Host) error {
			if i == 1 {
				infos, err := peers(ctx, ps[i], ns)
				require.NoError(t, err)
				require.Len(t, infos, 2)
				require.Equal(t, infos[0].ID, hs[0].ID())
			}
			return nil
		}).Go(func(ctx context.Context, i int, h host.Host) error {
		if i == 0 {
			err := hs[i].Close()
			require.NoError(t, err)
			ps[i].Close()
			require.NoError(t, err)
		} else {
			ttl, err := ps[i].Advertise(ctx, ns)
			require.NoError(t, err)
			require.LessOrEqual(t, ttl, 5*time.Minute,
				"should return TTL less or equal to default Tick of 5 minutes")
			require.GreaterOrEqual(t, ttl, 5*time.Minute/2,
				"should return TTL greater or equal to half of the default Tick of 5 minutes")
		}
		return nil
	}).Must(ctx, hs)
}

func TestPeX_Simulation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const (
		n = 8 // number of peers in cluster
	)

	var (
		newGossip = func(ns string) pex.GossipConfig {
			config := pex.DefaultGossipConfig
			config.Tick = time.Millisecond
			return config
		}

		sim      = mx.New(ctx)
		hs       = sim.MustHostSet(ctx, n)
		ps       = make([]*pex.PeerExchange, len(hs))
		b        = make(boot.StaticAddrs, len(hs))
		finished atomic.Value
	)

	finished.Store(false)

	defer mx.Go(func(ctx context.Context, i int, h host.Host) error {
		ps[i].Close()
		return hs[i].Close()
	}).Must(ctx, hs)

	ns := "simulation"

	err := mx.
		Go(func(ctx context.Context, i int, h host.Host) (err error) {
			b[i] = *host.InfoFromHost(h)
			return
		}).
		Go(func(ctx context.Context, i int, h host.Host) (err error) {
			ps[i], err = pex.New(ctx, h, pex.WithGossip(newGossip))
			return
		}).
		// start advertiser loop
		Go(func(ctx context.Context, i int, h host.Host) error {
			next, err := ps[i].Advertise(ctx, ns)
			if err != nil {
				return err
			}
			go func() {
				for {
					select {
					case <-time.After(next):
						next, err = ps[i].Advertise(ctx, ns)
						if !finished.Load().(bool) {
							require.NoError(t, err)
						}
					case <-ctx.Done():
						return
					}
				}
			}()

			return nil
		}).
		Err(ctx, hs)

	require.NoError(t, err)

	timer := time.NewTimer(5 * time.Second)
	<-timer.C
	finished.Store(true)
}
