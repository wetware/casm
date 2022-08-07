//go:build !race
// +build !race

package pex_test

import (
	"context"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p-core/host"
	"github.com/stretchr/testify/require"
	"github.com/wetware/casm/pkg/pex"
)

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
		// Arrange peers in a line topology
		func(i int, h host.Host) (err error) {
			if i > 0 {
				ps[i], err = pex.New(h, pex.WithBootstrapPeers(*host.InfoFromHost(hs[i-1])))
			} else {
				ps[i], err = pex.New(h)
			}

			return
		},
		// Advertise - TTL sampled in interval [5 10) ms
		func(i int, h host.Host) error {
			go func() {
				var next = time.Duration(0)
				for {
					select {
					case <-time.After(next):
						ps[i].Advertise(ctx, ns)
					case <-ctx.Done():
						return
					}
				}
			}()
			return nil
		})
	require.NoError(t, err, "must set up initial topology")

	require.Eventually(t, func() bool {
		infos, err := peers(ctx, ps[0], ns)
		require.NoError(t, err)
		return len(infos) == min(pex.DefaultMaxView, n-1)
	}, time.Second*5, time.Millisecond*10)
}

func BenchmarkAdvertise(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const (
		n  = 8
		ns = "benchmark-advertise"
	)

	local := newTestHost()
	defer local.Close()

	remote := newTestHost()
	defer remote.Close()

	pxLocal, _ := pex.New(local, pex.WithBootstrapPeers(*host.InfoFromHost(remote)))
	defer pxLocal.Close()

	pxRemote, _ := pex.New(remote)
	defer pxRemote.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := pxLocal.Advertise(ctx, ns)
		if err != nil {
			panic(err)
		}
	}
}
