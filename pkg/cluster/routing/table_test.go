package routing_test

import (
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wetware/casm/pkg/cluster/pulse"
	"github.com/wetware/casm/pkg/cluster/routing"
)

func TestRoutingTable_upsert(t *testing.T) {
	t.Parallel()

	table := routing.New(t0)

	rec := &record{}
	assert.True(t, table.Upsert(rec),
		"should ACCEPT new record")
	assert.False(t, table.Upsert(rec),
		"should REJECT duplicate record")

	rec2 := &record{id: rec.id, ins: rec.ins, seq: 1}
	assert.True(t, table.Upsert(rec2),
		"should ACCEPT matching instance id and higher sequence")
	rec3 := &record{id: rec.id, seq: 2}
	assert.False(t, table.Upsert(rec3),
		"should REJECT non-matching instance id and higer sequence")

	rec4 := &record{id: rec.id, ins: rec.ins, seq: 2}
	assert.True(t, table.Upsert(rec4),
		// This is the 'tie-breaker' heuristic;  When instance IDs differ,
		// but sequence is identical, assume the record being passed to Upsert
		// is the most recent.
		"should ACCEPT non-matching instance id and matching sequence")
}

func TestAdvance(t *testing.T) {
	t.Parallel()

	table := routing.New(t0)
	recs := []*record{
		{ttl: time.Millisecond},
		{ttl: time.Millisecond * 10},
		{ttl: time.Millisecond * 10},
		{ttl: time.Millisecond * 10},
		{ttl: time.Millisecond * 100},
		{ttl: time.Millisecond * 100},
	}

	for _, rec := range recs {
		require.True(t, table.Upsert(rec), "must upsert record")
	}

	it, err := table.NewQuery().Get(all{}) // query whole table
	require.NoError(t, err, "query should succeed")
	require.NotNil(t, it, "iterator should not be nil")
	require.Equal(t, len(recs), countRecords(it), "should contain all four records")

	t.Run("DropOne", func(t *testing.T) {
		table.Advance(t0.Add(time.Millisecond + 1)) // HACK:  LowerBound is '<'

		it, err := table.NewQuery().Get(all{}) // query whole table
		require.NoError(t, err, "query should succeed")
		require.NotNil(t, it, "iterator should not be nil")
		assert.Equal(t, len(recs)-1, countRecords(it), "should drop one record")
	})

	t.Run("DropTwo", func(t *testing.T) {
		table.Advance(t0.Add(time.Millisecond*10 + 1)) // HACK:  LowerBound is '<'

		it, err := table.NewQuery().Get(all{}) // query whole table
		require.NoError(t, err, "query should succeed")
		require.NotNil(t, it, "iterator should not be nil")
		assert.Equal(t, len(recs)-4, countRecords(it), "should drop one record")
	})

	t.Run("DropRemaining", func(t *testing.T) {
		table.Advance(t0.Add(time.Hour))

		it, err := table.NewQuery().Get(all{}) // query whole table
		require.NoError(t, err, "query should succeed")
		require.NotNil(t, it, "iterator should not be nil")
		require.Nil(t, it.Next(), "should drop all remaining records")
	})
}

func BenchmarkRoutingTable_upsert(b *testing.B) {
	var (
		recs  []*benchmarkRecord
		table = routing.New(t0)
	)

	b.Run("Insert", func(b *testing.B) {
		recs = make([]*benchmarkRecord, b.N)
		for i := range recs {
			recs[i] = newBenchmarkRecord()
		}

		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			if ok := table.Upsert(recs[i]); !ok {
				b.Fatalf("%d", i)
				b.FailNow()
			}
		}
	})

	b.Run("Drop", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if ok := table.Upsert(recs[i%len(recs)]); ok {
				b.Fatalf("%d", i)
				b.FailNow()
			}
		}
	})

	b.Run("Re-Insert", func(b *testing.B) {
		/*
		 * This benchmark tests performance of an Upsert on a record that is
		 * already present in the routing table, but whose incoming sequence
		 * number is higher, and therefore replaces the existing record.  In
		 * cluster environments, the overwhelming majority of heartbeats will
		 * fall under into this category.
		 */
		rec := newBenchmarkRecord()
		recs := make([]benchmarkRecord, b.N)
		for i := range recs {
			recs[i] = *rec
			recs[i].seq = uint64(i)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = table.Upsert(&recs[i])
		}
	})

	b.Run("Ecological", func(b *testing.B) {
		/*
		 * This benchmarks attempts to replicate the access patterns found in
		 * real-world deployments, where most records point to hosts already
		 * found in the routing table. Optimizations should target this common
		 * case.
		 */

		table := routing.New(t0)
		recs := newPopulation(b, .01) // 1% new records

		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			_ = table.Upsert(recs[i])
		}
	})
}

func newPopulation(b *testing.B, prop float64) []*benchmarkRecord {
	// recs := make([]*benchmarkRecord, b.N)
	nnew := int(float64(b.N)*prop) + 1
	peers := make([]*benchmarkRecord, int(float64(b.N)*.05)+1) // number of peers in cluster

	// create the cluster peers
	for i := range peers {
		peers[i] = newBenchmarkRecord()
	}

	recs := make([]*benchmarkRecord, 0, b.N)

	// populate the first nnew records in recs with new records
	for i := 0; i < nnew; i++ {
		recs = append(recs, newBenchmarkRecord())
	}

	// fill the rest of recs with a random sampling of cluster peers
	for i := 0; i < (b.N - nnew); i++ {
		recs = append(recs, peers[i%len(peers)])
	}

	// shuffle
	rand.Shuffle(len(recs), func(i, j int) {
		recs[i], recs[j] = recs[j], recs[i]
	})

	return recs
}

type benchmarkRecord struct {
	*pulse.Heartbeat
	seq     uint64
	id      peer.ID
	idBytes []byte
}

func newBenchmarkRecord() *benchmarkRecord {
	hb := pulse.NewHeartbeat()

	id := newPeerID()
	if err := hb.SetHost(id.String()[:16]); err != nil {
		panic(err)
	}

	idBytes, err := id.MarshalBinary()
	if err != nil {
		panic(err)
	}

	return &benchmarkRecord{
		Heartbeat: (*pulse.Heartbeat)(&hb),
		id:        id,
		idBytes:   idBytes,
	}
}

func (r *benchmarkRecord) Seq() uint64                 { return r.seq }
func (r *benchmarkRecord) Peer() peer.ID               { return r.id }
func (r *benchmarkRecord) PeerBytes() ([]byte, error)  { return r.idBytes, nil }
func (r *benchmarkRecord) Meta() (routing.Meta, error) { return r.Heartbeat.Meta() }

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

func countRecords(it routing.Iterator) (i int) {
	for it.Next() != nil {
		i++
	}

	return
}

type all struct{}

func (all) String() string             { return "id" }
func (all) PeerBytes() ([]byte, error) { return nil, nil }
func (all) Match(routing.Record) bool  { return true }
