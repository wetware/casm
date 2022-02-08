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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sim := mx.New(ctx)
	h1 := sim.MustHost(ctx)
	defer h1.Close()
	h2 := sim.MustHost(ctx)
	defer h2.Close()
	waitReady(h1)
	waitReady(h2)

	addr, _ := net.ResolveUDPAddr("udp4", multicastAddr)

	a1, err := survey.New(h1, addr)
	require.NoError(t, err)
	defer a1.Close()

	a2, err := survey.New(h2, addr)
	require.NoError(t, err)
	defer a2.Close()

	gradual := survey.GradualSurveyor{Surveyor: a1, Min: findPeersTTL, Max: findPeersTTL}

	a1.Advertise(ctx, testNs, discovery.TTL(advertiseTTL))
	a2.Advertise(ctx, testNs, discovery.TTL(advertiseTTL))

	infos := make([]peer.AddrInfo, 0)

	ctxTtl, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	finder, err := gradual.FindPeers(ctxTtl, testNs)
	require.NoError(t, err)

	for info := range finder {
		infos = append(infos, info)
	}

	require.Len(t, infos, 1)
	require.Equal(t, infos[0].ID, h2.ID())
}
