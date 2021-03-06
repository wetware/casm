package routing_test

import (
	"crypto/rand"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/wetware/casm/pkg/cluster/routing"
)

var t0 = time.Date(2020, 4, 9, 8, 0, 0, 0, time.UTC)

func TestRoutingTable(t *testing.T) {
	t.Parallel()

	var (
		rt = routing.New()
		id = newPeerID()
	)

	assert.NotPanics(t, func() { rt.Advance(t0) },
		"advancing an empty filter should not panic.")

	assert.False(t, contains(rt, id),
		"canary failed:  ID should not be present in empty filter.")

	assert.True(t, rt.Upsert(record(id, 0)),
		"upsert of new ID should succeed.")

	assert.True(t, contains(rt, id),
		"filter should contain ID %s after INSERT.", id)

	it := rt.Iter()
	assert.True(t, it.More())
	assert.True(t, it.Next())
	assert.Equal(t, t0.Add(time.Second), it.Deadline())

	assert.True(t, rt.Upsert(record(id, 3)),
		"upsert of existing ID with higher sequence number should succeed.")

	assert.True(t, contains(rt, id),
		"filter should contain ID %s after UPDATE", id)

	assert.False(t, rt.Upsert(record(id, 1)),
		"upsert of existing ID with lower sequence number should fail.")

	assert.True(t, contains(rt, id),
		"filter should contain ID %s after FAILED UPDATE.", id)

	assert.Contains(t, peers(rt), id,
		"ID should appear in peer.IDSlice")

	rt.Advance(t0.Add(time.Millisecond * 100))
	assert.True(t, contains(rt, id),
		"advancing by less than the TTL amount should NOT cause eviction.")

	rt.Advance(t0.Add(time.Second))
	assert.False(t, contains(rt, id),
		"advancing by more than the TTL amount should cause eviction")
}

type testRecord struct {
	id  peer.ID
	seq uint64
	ttl time.Duration
}

func record(id peer.ID, seq uint64) testRecord {
	return testRecord{
		id:  id,
		seq: seq,
		ttl: time.Second,
	}
}

func (r testRecord) Peer() peer.ID      { return r.id }
func (r testRecord) Seq() uint64        { return r.seq }
func (r testRecord) TTL() time.Duration { return r.ttl }

func contains(rt *routing.Table, id peer.ID) bool {
	_, ok := rt.Lookup(id)
	return ok
}

func peers(rt *routing.Table) (ps peer.IDSlice) {
	for it := rt.Iter(); it.Next(); {
		ps = append(ps, it.Record().Peer())
	}
	return
}

func newPeerID() peer.ID {
	sk, _, err := crypto.GenerateECDSAKeyPair(rand.Reader)
	if err != nil {
		panic(err)
	}

	id, err := peer.IDFromPrivateKey(sk)
	if err != nil {
		panic(err)
	}

	return id
}
