package net_test

import (
	"context"
	"io"
	"math/rand"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	swarm "github.com/libp2p/go-libp2p-swarm"
	inproc "github.com/lthibault/go-libp2p-inproc-transport"
	"github.com/stretchr/testify/require"
	"github.com/wetware/casm/pkg/net"
	"golang.org/x/sync/errgroup"
)

func TestJoin(t *testing.T) {
	t.Parallel()

	t.Run("Singleton", func(t *testing.T) {
		t.Parallel()

		var env = inproc.NewEnv()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		h, err := newTestHost(ctx, env)
		require.NoError(t, err)
		defer h.Close()

		o, err := net.New(h)
		require.NoError(t, err)

		err = o.Join(ctx, net.StaticAddrs{*host.InfoFromHost(h)})
		require.ErrorIs(t, err, net.ErrNoPeers)
		require.ErrorIs(t, err, swarm.ErrDialToSelf)
	})

	t.Run("WithPeers", func(t *testing.T) {
		t.Parallel()

		const n = 3

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		tc, err := newTestCluster(ctx, n)
		require.NoError(t, err)
		defer tc.Close()

		err = tc.ConnectStar(ctx)
		require.NoError(t, err)

		require.Eventually(t, func() bool {
			for _, o := range tc.os {
				if size := len(o.Stat()); size != n-1 {
					return false
				}
			}
			return true
		}, time.Millisecond*100, time.Millisecond*10)
	})
}

func TestEventsEmitted(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tc, err := newTestCluster(ctx, 2)
	require.NoError(t, err)
	defer tc.Close()

	sub0, err := tc.hs[0].EventBus().Subscribe(new(net.EvtState))
	require.NoError(t, err)
	defer sub0.Close()

	sub1, err := tc.hs[1].EventBus().Subscribe(new(net.EvtState))
	require.NoError(t, err)
	defer sub1.Close()

	t.Run("EventJoin", func(t *testing.T) {
		require.NoError(t, tc.ConnectLine(ctx))

		t.Run("Host0", func(t *testing.T) {
			select {
			case v, ok := <-sub0.Out():
				require.True(t, ok, "subscription prematurely closed")

				ev := v.(net.EvtState)
				require.Equal(t, net.EventJoined, ev.Event)
				require.Contains(t, ev.Edges(), tc.hs[1].ID())

			case <-time.After(time.Millisecond * 10):
				t.Errorf("did not receive event after %s", time.Millisecond*10)
			}
		})

		t.Run("Host1", func(t *testing.T) {
			select {
			case v, ok := <-sub1.Out():
				require.True(t, ok, "subscription prematurely closed")

				ev := v.(net.EvtState)
				require.Equal(t, net.EventJoined, ev.Event)
				require.Contains(t, ev.Edges(), tc.hs[0].ID())

			case <-time.After(time.Millisecond * 10):
				t.Errorf("did not receive event after %s", time.Millisecond*10)
			}
		})
	})

	t.Run("EventLeave", func(t *testing.T) {
		for _, s := range tc.hs[0].Network().Conns()[0].GetStreams() {
			if strings.HasPrefix(string(s.Protocol()), string(net.BaseProto)) {
				require.NoError(t, s.Reset())
			}
		}
		t.Run("Host0", func(t *testing.T) {
			select {
			case v, ok := <-sub0.Out():
				require.True(t, ok, "subscription prematurely closed")

				ev := v.(net.EvtState)
				require.Equal(t, net.EventLeft, ev.Event)
				require.NotContains(t, ev.Edges(), tc.hs[1].ID())

			case <-time.After(time.Millisecond * 10):
				t.Errorf("did not receive event after %s", time.Millisecond*10)
			}
		})

		t.Run("Host1", func(t *testing.T) {
			select {
			case v, ok := <-sub1.Out():
				require.True(t, ok, "subscription prematurely closed")

				ev := v.(net.EvtState)
				require.Equal(t, net.EventLeft, ev.Event)
				require.NotContains(t, ev.Edges(), tc.hs[0].ID())

			case <-time.After(time.Millisecond * 10):
				t.Errorf("did not receive event after %s", time.Millisecond*10)
			}
		})
	})
}

type testCluster struct {
	env inproc.Env
	hs  hostSlice
	os  []*net.Overlay
}

func newTestCluster(ctx context.Context, n int) (tc testCluster, err error) {
	tc.env = inproc.NewEnv()
	if tc.hs, err = newHostSlice(ctx, tc.env, n); err == nil {
		tc.os, err = tc.hs.ToOverlay()
	}

	return
}

func (tc testCluster) Close() error {
	var (
		g  errgroup.Group
		wg sync.WaitGroup
	)

	wg.Add(len(tc.os))
	for _, o := range tc.os {
		g.Go(func(c io.Closer) func() error {
			return func() error {
				defer wg.Done()
				return c.Close()
			}
		}(o))
	}

	g.Go(func() error {
		wg.Wait()
		return tc.hs.Close()
	})

	return g.Wait()
}

func (tc testCluster) ConnectLine(ctx context.Context) (err error) {
	for i, h := range tc.hs {
		if i == 0 {
			continue
		}

		if err = tc.os[i-1].Join(ctx, net.StaticAddrs{*host.InfoFromHost(h)}); err != nil {
			break
		}
	}
	return
}

func (tc testCluster) ConnectStar(ctx context.Context) (err error) {
	for _, o := range tc.os {
		if err = o.Join(ctx, tc.hs); err != nil {
			break
		}
	}
	return
}

func newTestHost(ctx context.Context, env inproc.Env) (host.Host, error) {
	return libp2p.New(ctx, libp2p.NoTransports,
		libp2p.Transport(inproc.New(inproc.WithEnv(env))),
		libp2p.ListenAddrStrings("/inproc/~"))
}

type hostSlice []host.Host

func newHostSlice(ctx context.Context, env inproc.Env, n int) (hostSlice, error) {
	hs := make(hostSlice, 0, n)
	for i := 0; i < n; i++ {
		h, err := newTestHost(ctx, env)

		if err == nil {
			hs = append(hs, h)
			continue
		}

		// best-and-suspenders:  close all hosts to free up the env
		hs.Close()

		return nil, err
	}

	return hs, nil
}

func (hs hostSlice) Close() error {
	var g errgroup.Group
	for _, h := range hs {
		g.Go(h.Close)
	}
	return g.Wait()
}

func (hs hostSlice) Len() int           { return len(hs) }
func (hs hostSlice) Swap(i, j int)      { hs[i], hs[j] = hs[j], hs[i] }
func (hs hostSlice) Less(i, j int) bool { return hs[i].ID() < hs[j].ID() }

func (hs hostSlice) FindPeers(ctx context.Context, _ string, opt ...discovery.Option) (<-chan peer.AddrInfo, error) {
	hh := make(hostSlice, hs.Len())
	copy(hh, hs)
	rand.Shuffle(hh.Len(), hh.Swap)

	d := make(net.StaticAddrs, hh.Len())
	for i, h := range hh {
		d[i] = *host.InfoFromHost(h)
	}

	return d.FindPeers(ctx, "", opt...)
}

func (hs hostSlice) ToOverlay() (os []*net.Overlay, err error) {
	os = make([]*net.Overlay, len(hs))
	for i, h := range hs {
		if os[i], err = net.New(h); err != nil {
			hs.Close()
			break
		}
	}

	return
}
