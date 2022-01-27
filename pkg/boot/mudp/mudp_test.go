package mudp

import (
	"context"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/require"
	mx "github.com/wetware/matrix/pkg"
)

const (
	testNs       = "casm/mudp"
	advertiseTTL = time.Minute
	findPeersTTL = 10 * time.Millisecond
)

type MockDisc struct {
	h host.Host
}

func (md *MockDisc) FindPeers(ctx context.Context, ns string, opts ...discovery.Option) (<-chan peer.AddrInfo, error) {
	finder := make(chan peer.AddrInfo, 1)
	finder <- peer.AddrInfo{ID: md.h.ID(), Addrs: md.h.Addrs()}
	close(finder)
	return finder, nil

}

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

	a1, err := NewMudp(h1, &MockDisc{h: h1})
	require.NoError(t, err)
	defer a1.Close()

	a2, err := NewMudp(h2, &MockDisc{h: h2})
	require.NoError(t, err)
	defer a2.Close()

	a1.Advertise(ctx, testNs, discovery.TTL(advertiseTTL))
	a2.Advertise(ctx, testNs, discovery.TTL(advertiseTTL))

	infos := make([]peer.AddrInfo, 0)
	for i := uint8(1); i < uint8(255) && len(infos) == 0; i += 5 {
		finder, err := a1.FindPeers(ctx, testNs, discovery.TTL(findPeersTTL), Distance(i))
		require.NoError(t, err)

		for info := range finder {
			infos = append(infos, info)
		}
	}

	require.True(t, len(infos) == 1)
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

	a1, err := NewMudp(h1, &MockDisc{h: h1})
	require.NoError(t, err)
	defer a1.Close()

	a2, err := NewMudp(h2, &MockDisc{h: h2})
	require.NoError(t, err)
	defer a2.Close()

	infos := make([]peer.AddrInfo, 0)
	for i := uint8(1); i < uint8(255) && len(infos) == 0; i += 5 {
		finder, err := a1.FindPeers(ctx, testNs, discovery.TTL(findPeersTTL), Distance(i))
		require.NoError(t, err)

		for info := range finder {
			infos = append(infos, info)
		}
	}

	require.True(t, len(infos) == 0)
}

func waitReady(h host.Host) {
	sub, err := h.EventBus().Subscribe(new(event.EvtLocalAddressesUpdated))
	if err != nil {
		panic(err)
	}
	defer sub.Close()

	<-sub.Out()
}
