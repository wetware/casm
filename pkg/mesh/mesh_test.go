package mesh_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/test"
	swarm "github.com/libp2p/go-libp2p-swarm"

	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"

	mock_libp2p "github.com/wetware/casm/internal/mock/libp2p"
	"github.com/wetware/casm/pkg/mesh"
)

var addrs mesh.StaticAddrs

func init() {
	for _, ma := range test.GenerateTestAddrs(1) {
		id, err := test.RandPeerID()
		if err != nil {
			panic(err)
		}

		addr := multiaddr.Join(ma,
			multiaddr.StringCast(fmt.Sprintf("/p2p/%s", id)))

		info, err := peer.AddrInfoFromP2pAddr(addr)
		if err != nil {
			panic(fmt.Errorf("%w: %s", err, ma))
		}

		addrs = append(addrs, *info)
	}
}

func TestJoin(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	network := mock_libp2p.NewMockNetwork(ctrl)
	network.EXPECT().
		Notify(gomock.Any()).
		Times(1)

	host := mock_libp2p.NewMockHost(ctrl)
	host.EXPECT().
		ID().
		Return(addrs[0].ID).
		AnyTimes()
	host.EXPECT().
		Network().
		Return(network).
		Times(1)
	host.EXPECT().
		SetStreamHandler(gomock.Any(), gomock.Any()).
		Times(2)
	host.EXPECT().
		RemoveStreamHandler(gomock.Any()).
		Times(2)
	host.EXPECT().
		Connect(gomock.Any(), addrs[0]).
		Return(nil).
		Times(1)
	host.EXPECT().
		NewStream(gomock.Any(), addrs[0].ID, mesh.JoinProto).
		Return(nil, swarm.ErrDialToSelf).
		Times(1)

	n := mesh.New(host, mesh.WithNamespace("casm.test.mesh"))
	defer func() { require.NoError(t, n.Close()) }()

	err := n.Join(ctx, addrs[:1], discovery.Limit(1))
	require.ErrorIs(t, err, mesh.ErrNoPeers)
}
