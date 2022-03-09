package pex_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	inproc "github.com/lthibault/go-libp2p-inproc-transport"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	mock_libp2p "github.com/wetware/casm/internal/mock/libp2p"
	"github.com/wetware/casm/pkg/boot"
	"github.com/wetware/casm/pkg/pex"
)

func init() {
	pex.DefaultGossipConfig.Tick = time.Millisecond
	pex.DefaultGossipConfig.Timeout = time.Millisecond * 10
}

func TestPeX_Init(t *testing.T) {
	t.Parallel()
	t.Helper()

	ns := "init"

	t.Run("Fail to find unadvertised ns", func(t *testing.T) {
		t.Parallel()

		h := newTestHost()

		px, err := pex.New(context.Background(), h)
		require.NoError(t, err)

		peers, err := px.FindPeers(context.Background(), ns)
		require.Error(t, err)
		require.Nil(t, peers)
	})

	t.Run("Fail_no_addrs", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		h := mock_libp2p.NewMockHost(ctrl)
		h.EXPECT().
			Addrs().
			Return([]ma.Multiaddr{}).
			Times(1)

		_, err := pex.New(context.Background(), h)
		require.EqualError(t, err, "host not accepting connections")
	})
}

func TestPeX_Bootstrap(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hs := makeHosts(2)
	defer closeAll(t, hs)

	ps := make([]*pex.PeerExchange, len(hs))
	is := make([]peer.AddrInfo, len(hs))

	const ns = "bootstrap"

	err := compose(hs,
		func(i int, h host.Host) (err error) {
			is[i] = *host.InfoFromHost(h)
			ps[i], err = pex.New(ctx, h)
			return
		},
		func(i int, h host.Host) error {
			if i == 0 {
				is[1] = *host.InfoFromHost(h)
			} else {
				is[0] = *host.InfoFromHost(h)
			}
			return nil
		},
		func(i int, h host.Host) error {
			if i != 0 {
				_, err := ps[i].Advertise(ctx, ns)
				require.NoError(t, err)
			}
			return nil
		},
		func(i int, h host.Host) (err error) {
			if i == 0 {
				err = ps[i].Bootstrap(ctx, ns, is[i])
			}

			return
		},
		func(i int, h host.Host) error {
			// TODO:  proper synchronization to ensure the 'Join()' call has completed
			time.Sleep(time.Millisecond * 10)

			ch, err := ps[i].FindPeers(ctx, ns)
			require.NoError(t, err)

			info, ok := <-ch
			require.True(t, ok)
			require.Equal(t, is[i].ID, info.ID)
			return nil
		})
	require.NoError(t, err, "must compose")
}

func TestPeX_Advertise(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hs := makeHosts(8)
	defer closeAll(t, hs)

	ps := make([]*pex.PeerExchange, len(hs))
	as := make(boot.StaticAddrs, len(hs))

	const ns = "advertise"

	err := compose(hs,
		func(i int, h host.Host) (err error) {
			as[i] = *host.InfoFromHost(h)
			return
		},
		func(i int, h host.Host) (err error) {
			b := make(boot.StaticAddrs, 0, len(as)-1)
			for _, info := range as {
				if info.ID != h.ID() {
					b = append(b, info)
				}
			}

			ps[i], err = pex.New(ctx, h, pex.WithDiscovery(b))
			return
		})
	require.NoError(t, err)

	_, err = ps[0].Advertise(ctx, ns)
	require.NoError(t, err)
}

func TestPeX_SingleNode(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const ns = "single-node"

	h := newTestHost()

	px, err := pex.New(ctx, h)
	require.NoError(t, err, "should construct PeerExchange")
	require.NotNil(t, px, "should return PeerExchange")

	ttl, err := px.Advertise(ctx, ns)
	require.NoError(t, err)
	require.LessOrEqual(t, ttl, pex.DefaultGossipConfig.Tick,
		"should return TTL less or equal to tick (%s)", pex.DefaultGossipConfig.Tick)
	require.GreaterOrEqual(t, ttl, pex.DefaultGossipConfig.Tick/2,
		"should return TTL greater or equal to half of tick (%s)", pex.DefaultGossipConfig.Tick)

	finder, err := px.FindPeers(ctx, ns)
	require.NoError(t, err)

	_, ok := <-finder
	require.False(t, ok, "shouldn't find any peer, the channel should close directly")
}

func TestPeX_TwoNodes(t *testing.T) {
	t.Parallel()

	const ns = "two-nodes"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hs := makeHosts(2)
	defer closeAll(t, hs)

	ps := make([]*pex.PeerExchange, len(hs))
	err := compose(hs,
		func(i int, h host.Host) (err error) {
			if i == 0 {
				ps[i], err = pex.New(ctx, h)
			} else {
				ps[i], err = pex.New(ctx, h, pex.WithBootstrapPeers(*host.InfoFromHost(hs[0])))
			}

			return
		},
		func(i int, h host.Host) error {
			_, err := ps[i].Advertise(ctx, ns)
			return err
		})
	require.NoError(t, err)

	assert.Eventually(t, func() bool {
		infos, err := peers(ctx, ps[1], ns)
		return assert.NoError(t, err) && len(infos) == 2 && infos[0].ID == hs[0].ID()
	}, time.Second*5, time.Millisecond*10)
}

func TestPex_NNodes(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const (
		n  = 8
		ns = "n-nodes"
	)

	hs := makeHosts(n)
	defer closeAll(t, hs)

	ps := make([]*pex.PeerExchange, len(hs))

	err := compose(hs,
		func(i int, h host.Host) (err error) {
			ps[i], err = pex.New(ctx, h)
			return
		},
		func(i int, h host.Host) error {
			_, err := ps[i].Advertise(ctx, ns)
			return err
		},
		func(i int, h host.Host) error {
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
		})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		infos, err := peers(ctx, ps[0], ns)
		require.NoError(t, err)
		return len(infos) == min(pex.DefaultMaxView, n-1)
	}, time.Second, time.Millisecond)
}

func TestPeX_DisconnectedNode(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hs := makeHosts(2)
	defer closeAll(t, hs)

	ps := make([]*pex.PeerExchange, len(hs))

	const ns = "disconnected-node"

	err := compose(hs,
		func(i int, h host.Host) (err error) {
			if i == 0 {
				ps[i], err = pex.New(ctx, h)
			} else {
				ps[i], err = pex.New(ctx, h,
					pex.WithBootstrapPeers(*host.InfoFromHost(hs[0])))
			}
			return
		},
		func(i int, h host.Host) error {
			ttl, err := ps[i].Advertise(ctx, ns)
			if err == nil {
				return validateTTL(ttl)
			}
			return err
		},
		func(i int, h host.Host) error {
			time.Sleep(time.Millisecond)

			if i == 1 {
				infos, err := peers(ctx, ps[i], ns)
				require.NoError(t, err)
				require.Len(t, infos, 2)
				require.Equal(t, infos[0].ID, hs[0].ID())
			}
			return nil
		},
		func(i int, h host.Host) error {
			if i == 0 {
				defer ps[i].Close()
				return hs[i].Close()
			}

			ttl, err := ps[i].Advertise(ctx, ns)
			if err == nil {
				return validateTTL(ttl)
			}
			return err
		})
	require.NoError(t, err)
}

func closeAll(t *testing.T, hs []host.Host) {
	hmap(hs, func(i int, h host.Host) error {
		assert.NoError(t, h.Close(), "should shutdown gracefully (index=%d)", i)
		return nil
	})
}

func compose(hs []host.Host, fs ...func(int, host.Host) error) (err error) {
	for _, f := range fs {
		if err = hmap(hs, f); err != nil {
			break
		}
	}

	return
}

func hmap(hs []host.Host, f func(i int, h host.Host) error) (err error) {
	for i, h := range hs {
		if err = f(i, h); err != nil {
			break
		}
	}
	return
}

func makeHosts(n int) []host.Host {
	hs := make([]host.Host, n)
	for i := range hs {
		hs[i] = newTestHost()
	}
	return hs
}

func newTestHost() host.Host {
	h, err := libp2p.New(
		libp2p.NoListenAddrs,
		libp2p.NoTransports,
		libp2p.Transport(inproc.New()),
		libp2p.ListenAddrStrings("/inproc/~"))
	if err != nil {
		panic(err)
	}

	return h
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

func validateTTL(ttl time.Duration) error {
	if ttl > pex.DefaultGossipConfig.Tick {
		return fmt.Errorf("TTL exceeds max interval between gossip rounds (%s)",
			pex.DefaultGossipConfig.Tick)
	}

	if ttl < pex.DefaultGossipConfig.Tick/2 {
		return fmt.Errorf("TTL must be at least half of interval between gossip rounds (%s)",
			pex.DefaultGossipConfig.Tick)
	}

	return nil
}
