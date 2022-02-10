package survey_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/require"
	"github.com/wetware/casm/pkg/boot/survey"
	mx "github.com/wetware/matrix/pkg"
)

func TestDiscoverGradual(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sim := mx.New(ctx)
	h1 := sim.MustHost(ctx)
	defer h1.Close()
	h2 := sim.MustHost(ctx)
	defer h2.Close()

	const multicastAddr = "228.8.8.8:8822"
	addr, _ := net.ResolveUDPAddr("udp4", multicastAddr)

	a1, err := survey.New(h1, addr)
	require.NoError(t, err)
	defer a1.Close()

	a2, err := survey.New(h2, addr)
	require.NoError(t, err)
	defer a2.Close()

	gradual := survey.GradualSurveyor{Surveyor: a1, Min: 50 * time.Millisecond, Max: 50 * time.Millisecond}

	a1.Advertise(ctx, testNs, discovery.TTL(advertiseTTL))
	a2.Advertise(ctx, testNs, discovery.TTL(advertiseTTL))

	infos := make([]peer.AddrInfo, 0)

	ctxTtl, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	finder, err := gradual.FindPeers(ctxTtl, testNs)
	require.NoError(t, err)

	for info := range finder {
		infos = append(infos, info)
	}

	require.True(t, 1 < len(infos))
	for _, info := range infos {
		require.Equal(t, info.ID, h2.ID())
	}
}
