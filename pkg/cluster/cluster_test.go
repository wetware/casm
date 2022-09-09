package cluster_test

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	logtest "github.com/lthibault/log/test"
	mock_cluster "github.com/wetware/casm/internal/mock/pkg/cluster"
	"github.com/wetware/casm/pkg/cluster"

	"github.com/stretchr/testify/assert"
)

func TestRouter(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	var canceled bool
	defer func() {
		assert.True(t, canceled, "should cancel topic relay")
	}()

	logger := logtest.NewMockLogger(ctrl)
	logger.EXPECT().
		With(gomock.Any()).
		Return(logger).
		Times(1)

	table := mock_cluster.NewMockRoutingTable(ctrl)
	table.EXPECT().
		Advance(gomock.AssignableToTypeOf(time.Time{})).
		AnyTimes()

	topic := mock_cluster.NewMockTopic(ctrl)
	topic.EXPECT().
		String().
		Return("casm").
		AnyTimes()
	topic.EXPECT().
		Relay().
		Return(func() { canceled = true }, nil).
		Times(1)
	boot := topic.EXPECT().
		Publish(gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)
	topic.EXPECT().
		Publish(gomock.Any(), gomock.Any()).
		After(boot).
		AnyTimes()

	router := cluster.Router{
		Log:          logger,
		Topic:        topic,
		RoutingTable: table,
	}
	defer router.Stop()

	err := router.Bootstrap(context.Background())
	assert.NoError(t, err, "bootstrap should succeed")
}

// func TestRouter(t *testing.T) {
// 	t.Parallel()

// 	ctx, cancel := context.WithCancel(context.Background())
// 	defer cancel()

// 	h0 := newTestHost()
// 	defer h0.Close()
// 	h1 := newTestHost()
// 	defer h1.Close()

// 	// host 0
// 	ps0, err := pubsub.NewGossipSub(ctx, h0,
// 		pubsub.WithDirectPeers([]peer.AddrInfo{*host.InfoFromHost(h1)}))
// 	require.NoError(t, err)

// 	n0, err := cluster.New(ps0, cluster.WithTTL(time.Millisecond*100))
// 	require.NoError(t, err)
// 	defer func() {
// 		require.NoError(t, n0.Close())
// 	}()

// 	// host 1
// 	ps1, err := pubsub.NewGossipSub(ctx, h1,
// 		pubsub.WithDirectPeers([]peer.AddrInfo{*host.InfoFromHost(h0)}))
// 	require.NoError(t, err)

// 	n1, err := cluster.New(ps1, cluster.WithTTL(time.Millisecond*100))
// 	require.NoError(t, err)
// 	defer func() {
// 		require.NoError(t, n1.Close())
// 	}()

// 	// test
// 	assert.Eventually(t,
// 		func() bool {
// 			p0 := peers(n0)
// 			p1 := peers(n1)

// 			t.Logf("c0: %s; c1: %s", p0, p1)

// 			return len(p0) == 2 && len(p1) == 2
// 		},
// 		time.Second*5, time.Millisecond*10,
// 		"peers should eventually be found in each other's views")
// }

// func TestModel_announce_join(t *testing.T) {
// 	t.Parallel()

// 	ctx, cancel := context.WithCancel(context.Background())
// 	defer cancel()

// 	const n = 8
// 	var (
// 		hs = makeHosts(n)
// 		as = make(boot.StaticAddrs, n)
// 		ps = make([]cluster.PubSub, n)
// 		ns = make([]*cluster.Node, n)
// 	)

// 	err := compose(hs,
// 		// Initialize static addresses
// 		func(i int, h host.Host) error {
// 			as[i] = *host.InfoFromHost(h)
// 			return nil
// 		},
// 		// Initialize pubsub
// 		func(i int, h host.Host) (err error) {
// 			ps[i], err = pubsub.NewGossipSub(ctx, h,
// 				pubsub.WithDirectPeers(as.Filter(not(h))))
// 			return
// 		},
// 		// Initialize cluster
// 		func(i int, h host.Host) (err error) {
// 			ns[i], err = cluster.New(ps[i],
// 				// Ensure only the initial join heartbeat is emitted
// 				cluster.WithTTL(time.Hour))
// 			return
// 		},
// 		// Bootstrap
// 		func(i int, h host.Host) error {
// 			return ns[i].Bootstrap(ctx)
// 		})
// 	require.NoError(t, err, "must set up cluster")
// 	defer closeAll(t, hs)

// 	assert.Eventually(t,
// 		func() bool {
// 			for _, m := range ns {
// 				if len(peers(m)) > 0 {
// 					return true
// 				}
// 			}
// 			return false
// 		},
// 		time.Second*5,
// 		time.Millisecond*10,
// 		"peers should receive each other's bootstrap messages")
// }

// func TestModel_announce_live(t *testing.T) {
// 	t.Parallel()

// 	ctx, cancel := context.WithCancel(context.Background())
// 	defer cancel()

// 	const n = 8
// 	var (
// 		hs = makeHosts(n)
// 		as = make(boot.StaticAddrs, n)
// 		ps = make([]cluster.PubSub, n)
// 		ns = make([]*cluster.Node, n)
// 	)

// 	// Cluster setup
// 	err := compose(hs,
// 		func(i int, h host.Host) error {
// 			as[i] = *host.InfoFromHost(h)
// 			return nil
// 		},
// 		func(i int, h host.Host) (err error) {
// 			ps[i], err = pubsub.NewGossipSub(ctx, h,
// 				pubsub.WithDirectPeers(as.Filter(not(h))))
// 			return
// 		},
// 		func(i int, h host.Host) (err error) {
// 			ns[i], err = cluster.New(ps[i],
// 				cluster.WithTTL(time.Millisecond*150))
// 			return
// 		})
// 	require.NoError(t, err, "must set up cluster")
// 	defer closeAll(t, hs)

// 	assert.Eventually(t,
// 		func() bool {
// 			for _, info := range as {
// 				r, err := ns[n-1].NewQuery().Lookup(selectPeer(info.ID))
// 				if err != nil && r == nil {
// 					return false
// 				}
// 			}
// 			return true
// 		},
// 		time.Second*5,
// 		time.Millisecond*10,
// 		"last model should eventually receive heartbeats from all peers")
// }

// func not(h host.Host) func(peer.AddrInfo) bool {
// 	return func(info peer.AddrInfo) bool {
// 		return h.ID() != info.ID
// 	}
// }

// func peers(n *cluster.Node) (ps peer.IDSlice) {
// 	it, err := n.NewQuery().Iter(query.All())
// 	if err != nil {
// 		panic(err)
// 	}

// 	for r := it.Next(); r != nil; r = it.Next() {
// 		ps = append(ps, r.Peer())
// 	}

// 	return
// }

// func closeAll(t *testing.T, hs []host.Host) {
// 	hmap(hs, func(i int, h host.Host) error {
// 		assert.NoError(t, h.Close(), "should shutdown gracefully (index=%d)", i)
// 		return nil
// 	})
// }

// func compose(hs []host.Host, fs ...func(int, host.Host) error) (err error) {
// 	for _, f := range fs {
// 		if err = hmap(hs, f); err != nil {
// 			break
// 		}
// 	}

// 	return
// }

// func hmap(hs []host.Host, f func(i int, h host.Host) error) (err error) {
// 	for i, h := range hs {
// 		if err = f(i, h); err != nil {
// 			break
// 		}
// 	}
// 	return
// }

// func makeHosts(n int) []host.Host {
// 	hs := make([]host.Host, n)
// 	for i := range hs {
// 		hs[i] = newTestHost()
// 	}
// 	return hs
// }

// func newTestHost() host.Host {
// 	h, err := libp2p.New(
// 		libp2p.NoListenAddrs,
// 		libp2p.NoTransports,
// 		libp2p.Transport(inproc.New()),
// 		libp2p.ListenAddrStrings("/inproc/~"))
// 	if err != nil {
// 		panic(err)
// 	}

// 	return h
// }

// func selectPeer(id peer.ID) query.Selector {
// 	return query.Select(peerIndex(id))
// }

// type peerIndex peer.ID

// func (peerIndex) String() string                { return "id" }
// func (peerIndex) Prefix() bool                  { return false }
// func (ix peerIndex) PeerBytes() ([]byte, error) { return []byte(ix), nil }
