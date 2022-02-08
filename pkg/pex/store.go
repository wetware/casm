package pex

import (
	"math/rand"
	"sort"
	"time"

	"capnproto.org/go/capnp/v3"
	ds "github.com/ipfs/go-datastore"
	nsds "github.com/ipfs/go-datastore/namespace"
	"github.com/ipfs/go-datastore/query"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/wetware/casm/internal/api/pex"
)

func init() { rand.Seed(time.Now().UnixNano()) }

// rootStore is a factory type that derives "child" datastores
// that are scoped to a namespace.
type rootStore struct {
	ds.Batching
	atomicRecord
}

// New gossipStore, scoped to ns.
func (rs *rootStore) New(ns string) gossipStore {
	return gossipStore{
		ns:           ns,
		store:        nsds.Wrap(rs.Batching, ds.NewKey(ns)),
		atomicRecord: &rs.atomicRecord,
	}
}

type gossipStore struct {
	ns    string
	store ds.Batching
	*atomicRecord
}

func (gs gossipStore) String() string { return gs.ns }

func (gs gossipStore) Loggable() map[string]interface{} {
	return map[string]interface{}{
		"ns": gs.ns,
	}
}

func (gs gossipStore) LoadRecords() (View, error) {
	// return all entries under the local instance's key prefix
	res, err := gs.store.Query(query.Query{
		Prefix: "/",
		Orders: []query.Order{randomOrder()},
	})
	if err != nil {
		return nil, err
	}

	es, err := res.Rest()
	if err != nil {
		return nil, err
	}

	recs := make(View, len(es))
	for i, entry := range es {
		recs[i] = new(GossipRecord) // TODO:  pool?

		msg, err := capnp.Unmarshal(entry.Value)
		if err != nil {
			return nil, err
		}

		if err = recs[i].ReadMessage(msg); err != nil {
			return nil, err
		}
	}

	return recs, nil
}

func (gs gossipStore) StoreRecords(old, new View) error {
	batch, err := gs.store.Batch()
	if err != nil {
		return err
	}

	for _, g := range old.diff(new) { // elems in 'old', but not in 'new'.
		if err = batch.Delete(g.Key()); err != nil {
			return err
		}
	}

	for _, g := range new {
		// Marshal *unpacked*, since we are (probably) not sending these bytes
		// over the network.  This avoids allocating an extra slice with each
		// call to Load(); we benefit from capnp's direct-access semantics.
		b, err := g.Message().Marshal()
		if err != nil {
			return err
		}

		if err = batch.Put(g.Key(), b); err != nil {
			return err
		}
	}

	if err = batch.Commit(); err != nil {
		err = gs.store.Sync(ds.NewKey("/"))
	}

	return err
}

func randomOrder() query.OrderByFunction {
	nonce := rand.Uint64()

	return func(a, b query.Entry) int {
		xa := lastUint64(a.Key) ^ nonce
		xb := lastUint64(b.Key) ^ nonce

		if xa > xb {
			return 1
		}
		if xa < xb {
			return -1
		}
		return 0
	}
}

func filter(f func(*GossipRecord) bool) func(View) View {
	return func(v View) View {
		filtered := make(View, 0, len(v))
		for _, g := range v {
			if f(g) {
				filtered = append(filtered, g)
			}
		}
		return filtered
	}
}

func isNot(self peer.ID) func(View) View {
	return filter(func(g *GossipRecord) bool {
		return self != g.PeerID
	})
}

func merged(tail View) func(View) View {
	return func(v View) View {
		return append(
			v.Bind(dedupe(tail, true)),
			tail.Bind(dedupe(v, false))...)
	}
}

func dedupe(other View, keepEqual bool) func(View) View {
	return func(v View) View {
		return v.Bind(filter(func(g *GossipRecord) bool {
			have, found := other.find(g.PeerID)
			if keepEqual {
				return !found || g.Seq > have.Seq || g.Hop() < have.Hop() ||
					(g.Seq == have.Seq && g.Hop() == have.Hop())
			} else {
				return !found || g.Seq > have.Seq || g.Hop() < have.Hop()
			}
		}))
	}
}

func shuffled() func(View) View {
	return func(v View) View {
		rand.Shuffle(len(v), v.Swap)
		return v
	}
}

func sorted() func(View) View {
	return func(v View) View {
		sort.Sort(v)
		return v
	}
}

func head(n int) func(View) View {
	if n < 0 {
		panic("n must be greater than zero.")
	}

	return func(v View) View {
		if n < len(v) {
			v = v[:n]
		}

		return v
	}
}

func tail(n int) func(View) View {
	if n < 0 {
		panic("n must be greater than zero.")
	}

	return func(v View) View {
		if n < len(v) {
			v = v[len(v)-n:]
		}

		return v
	}
}

func decay(d float64, maxDecay int) func(View) View {
	return func(v View) View {
		for len(v) > 0 && rand.Float64() < d && maxDecay > 0 {
			v = v[:len(v)-1]
			maxDecay--
		}
		return v
	}
}

type recordProvider interface {
	Record() *peer.PeerRecord
}

func appendLocal(rec recordProvider) func(View) View {
	return func(v View) View {
		var g = GossipRecord{
			PeerRecord: *rec.Record(),
		}

		_, s, err := capnp.NewMessage(capnp.SingleSegment(nil))
		if err != nil {
			panic(err)
		}

		g.g, err = pex.NewRootGossip(s)
		if err != nil {
			panic(err)
		}

		return append(v, &g)
	}
}
