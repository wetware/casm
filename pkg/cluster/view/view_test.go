package view_test

import (
	"context"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/golang/mock/gomock"

	mock_cluster "github.com/wetware/casm/internal/mock/pkg/cluster"
	mock_routing "github.com/wetware/casm/internal/mock/pkg/cluster/routing"
	"github.com/wetware/casm/pkg/cluster/routing"
	"github.com/wetware/casm/pkg/cluster/view"
)

var recs = []*record{
	{id: newPeerID()},
	{id: newPeerID()},
	{id: newPeerID()},
	{id: newPeerID()},
	{id: newPeerID()},
}

func TestView_Lookup(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	iter := mock_routing.NewMockIterator(ctrl)
	iter.EXPECT().
		Next().
		Return(recs[0]).
		Times(1)
	iter.EXPECT().
		Next().
		Return(recs[1]).
		Times(1) // <- called, but skipped due to query.First()

	snap := mock_routing.NewMockSnapshot(ctrl)
	snap.EXPECT().
		Get(gomock.Any()).
		Return(iter, nil).
		Times(1)

	table := mock_cluster.NewMockRoutingTable(ctrl)
	table.EXPECT().
		Snapshot().
		Return(snap).
		Times(1)

	server := view.Server{RoutingTable: table}
	client := server.View()
	defer client.Release()

	f, release := client.Lookup(ctx, view.All())
	require.NotNil(t, release)
	defer release()

	r, err := f.Record()
	require.NoError(t, err)
	require.NotNil(t, r)
	require.Equal(t, recs[0].Peer(), r.Peer())
}

func TestView_Iter(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	iter := mock_routing.NewMockIterator(ctrl)
	for _, r := range recs {
		iter.EXPECT().Next().Return(r).Times(1)
	}
	iter.EXPECT().Next().Return(nil).Times(1)

	snap := mock_routing.NewMockSnapshot(ctrl)
	snap.EXPECT().
		Get(gomock.Any()).
		Return(iter, nil).
		Times(1)

	table := mock_cluster.NewMockRoutingTable(ctrl)
	table.EXPECT().
		Snapshot().
		Return(snap).
		Times(1)

	server := view.Server{RoutingTable: table}
	client := server.View()
	defer client.Release()

	require.True(t, client.Client().IsValid(),
		"should not be nil capability")

	it, release := client.Iter(ctx, view.All())
	require.NotZero(t, it)
	require.NotNil(t, release)
	defer release()

	assert.NoError(t, it.Err(), "iterator should be valid")

	var got []routing.Record
	for r := range it.C {
		got = append(got, r)
	}

	for i, rec := range recs {
		assert.Equal(t, rec.Peer(), got[i].Peer(),
			"should match record %d", i)
	}

	require.NoError(t, it.Err(), "iterator should not encounter error")
}

type record struct {
	once sync.Once
	id   peer.ID
	seq  uint64
	ins  uint32
	host string
	meta routing.Meta
	ttl  time.Duration
}

func (r *record) init() {
	r.once.Do(func() {
		if r.id == "" {
			r.id = newPeerID()
		}

		if r.host == "" {
			r.host = newPeerID().String()[:16]
		}

		if r.ins == 0 {
			r.ins = rand.Uint32()
		}
	})
}

func (r *record) Peer() peer.ID {
	r.init()
	return r.id
}

func (r *record) Seq() uint64 { return r.seq }

func (r *record) Host() (string, error) {
	r.init()
	return r.host, nil
}

func (r *record) Instance() uint32 {
	r.init()
	return r.ins
}

func (r *record) TTL() time.Duration {
	if r.init(); r.ttl == 0 {
		return time.Second
	}

	return r.ttl
}

func (r *record) Meta() (routing.Meta, error) { return r.meta, nil }

func newPeerID() peer.ID {
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	sk, _, err := crypto.GenerateEd25519Key(rnd)
	if err != nil {
		panic(err)
	}

	id, err := peer.IDFromPrivateKey(sk)
	if err != nil {
		panic(err)
	}

	return id
}
