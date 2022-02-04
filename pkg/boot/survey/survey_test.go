package survey_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/require"
	"github.com/wetware/casm/pkg/boot/survey"
	mx "github.com/wetware/matrix/pkg"
)

const (
	testNs        = "casm/survey"
	advertiseTTL  = time.Minute
	findPeersTTL  = 10 * time.Millisecond
	multicastAddr = "224.0.1.241:3037"
)

func TestDiscover(t *testing.T) {
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

	a1.Advertise(ctx, testNs, discovery.TTL(advertiseTTL))
	a2.Advertise(ctx, testNs, discovery.TTL(advertiseTTL))

	infos := make([]peer.AddrInfo, 0)
	for i := uint8(1); i < uint8(255) && len(infos) == 0; i += 5 {
		finder, err := a1.FindPeers(ctx, testNs, discovery.TTL(findPeersTTL), survey.WithDistance(i))
		require.NoError(t, err)

		for info := range finder {
			infos = append(infos, info)
		}
	}

	require.Len(t, infos, 1)
	require.Equal(t, infos[0].ID, h2.ID())
}

func TestDiscoverNone(t *testing.T) {
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

	infos := make([]peer.AddrInfo, 0)
	for i := uint8(1); i < uint8(255) && len(infos) == 0; i += 5 {
		finder, err := a1.FindPeers(ctx, testNs, discovery.TTL(findPeersTTL), survey.WithDistance(i))
		require.NoError(t, err)

		for info := range finder {
			infos = append(infos, info)
		}
	}

	require.Len(t, infos, 0)
}

func TestClose(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sim := mx.New(ctx)
	h1 := sim.MustHost(ctx)
	defer h1.Close()
	waitReady(h1)

	addr, _ := net.ResolveUDPAddr("udp4", multicastAddr)

	a1, err := survey.New(h1, addr)
	require.NoError(t, err)

	_, err = a1.Advertise(ctx, testNs, discovery.TTL(advertiseTTL))
	require.NoError(t, err)

	a1.Close()

	_, err = a1.Advertise(ctx, testNs, discovery.TTL(advertiseTTL))
	require.Error(t, err, "closed")
}

func waitReady(h host.Host) {
	sub, err := h.EventBus().Subscribe(new(event.EvtLocalAddressesUpdated))
	if err != nil {
		panic(err)
	}
	defer sub.Close()

	<-sub.Out()
}
