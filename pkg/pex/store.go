package pex

import (
	"fmt"
	"math/rand"
	"sort"
	"time"

	"capnproto.org/go/capnp/v3"
	ds "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	"github.com/ipfs/go-datastore/query"
	"github.com/libp2p/go-libp2p-core/peer"
)

func init() { rand.Seed(time.Now().UnixNano()) }

// Gossip contains parameters for the PeX gossip algorithm.
type Gossip struct {
	C int     // maximum View size
	S int     // swapping amount
	P int     // protection amount
	D float64 // retention decay probability
}

type GossipStore struct {
	ns    string
	store ds.Batching
	g     Gossip
}

func NewGossipStore(ns string, store ds.Batching, g Gossip) GossipStore {
	return GossipStore{
		ns:    ns,
		store: namespace.Wrap(store, ds.NewKey(ns)),
		g:     g,
	}
}

func (gs GossipStore) String() string { return gs.ns }

func (gs GossipStore) Loggable() map[string]interface{} {
	return map[string]interface{}{
		"ns": gs.ns,
	}
}

func (gs GossipStore) LoadRecords() (gossipSlice, error) {
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

	recs := make(gossipSlice, len(es))
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

func (gs GossipStore) mtu() int64 { return int64(gs.g.C * mtu) }

func (gs GossipStore) tail() func(gossipSlice) gossipSlice {
	return tail(gs.g.P)
}

func (gs GossipStore) head() func(gossipSlice) gossipSlice {
	return head((gs.g.C / 2) - 1)
}

func (gs GossipStore) MergeAndStore(self peer.ID, local, remote gossipSlice) error {
	if err := remote.Validate(); err != nil {
		return err
	}

	// Remove duplicates and combine local and remote records
	newLocal := local.
		Bind(merged(remote)).
		Bind(isNot(self))

	// Apply swapping
	s := min(gs.g.S, max(len(newLocal)-gs.g.C, 0))
	newLocal = newLocal.
		Bind(tail(len(newLocal) - s)).
		Bind(sorted())

	// Apply retention
	r := min(min(gs.g.P, gs.g.C), len(newLocal))
	maxDecay := min(r, max(len(newLocal)-gs.g.C, 0))
	oldest := newLocal.Bind(tail(r)).Bind(decay(gs.g.D, maxDecay))

	//Apply random eviction
	c := gs.g.C - len(oldest)
	newLocal = newLocal.
		Bind(head(max(len(newLocal)-r, 0))).
		Bind(shuffled()).
		Bind(head(c))

	// Merge with oldest nodes
	newLocal = newLocal.
		Bind(merged(oldest))

	newLocal.incrHops()

	if err := gs.storeRecords(local, newLocal); err != nil {
		return err
	}

	return gs.store.Sync(ds.NewKey("/"))
}

func (gs GossipStore) storeRecords(old, new gossipSlice) error {
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

	return batch.Commit()
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

func filter(f func(*GossipRecord) bool) func(gossipSlice) gossipSlice {
	return func(gs gossipSlice) gossipSlice {
		filtered := make(gossipSlice, 0, len(gs))
		for _, g := range gs {
			if f(g) {
				filtered = append(filtered, g)
			}
		}
		return filtered
	}
}

func isNot(self peer.ID) func(gossipSlice) gossipSlice {
	return filter(func(g *GossipRecord) bool {
		return self != g.PeerID
	})
}

func merged(tail gossipSlice) func(gossipSlice) gossipSlice {
	return func(gs gossipSlice) gossipSlice {
		return append(
			gs.Bind(dedupe(tail, true)),
			tail.Bind(dedupe(gs, false))...)
	}
}

func dedupe(other gossipSlice, keepEqual bool) func(gossipSlice) gossipSlice {
	return func(gs gossipSlice) gossipSlice {
		return gs.Bind(filter(func(g *GossipRecord) bool {
			have, found := other.find(g)
			if keepEqual {
				return !found || g.Seq > have.Seq || g.Hop() < have.Hop() ||
					(g.Seq == have.Seq && g.Hop() == have.Hop())
			} else {
				return !found || g.Seq > have.Seq || g.Hop() < have.Hop()
			}
		}))
	}
}

func shuffled() func(gossipSlice) gossipSlice {
	return func(gs gossipSlice) gossipSlice {
		rand.Shuffle(len(gs), gs.Swap)
		return gs
	}
}

func sorted() func(gossipSlice) gossipSlice {
	return func(gs gossipSlice) gossipSlice {
		sort.Sort(gs)
		return gs
	}
}

func head(n int) func(gossipSlice) gossipSlice {
	if n < 0 {
		panic("n must be greater than zero.")
	}

	return func(gs gossipSlice) gossipSlice {
		if n < len(gs) {
			gs = gs[:n]
		}

		return gs
	}
}

func tail(n int) func(gossipSlice) gossipSlice {
	if n < 0 {
		panic("n must be greater than zero.")
	}

	return func(gs gossipSlice) gossipSlice {
		if n < len(gs) {
			gs = gs[len(gs)-n:]
		}

		return gs
	}
}

func decay(d float64, maxDecay int) func(gossipSlice) gossipSlice {
	return func(gs gossipSlice) gossipSlice {
		for len(gs) > 0 && rand.Float64() < d && maxDecay > 0 {
			gs = gs[:len(gs)-1]
			maxDecay--
		}
		return gs
	}
}

func appendLocal(m Mint) func(gossipSlice) gossipSlice {
	return func(gs gossipSlice) gossipSlice {
		var (
			g   GossipRecord
			err error
		)

		if g.g, err = newGossip(capnp.SingleSegment(nil)); err != nil {
			panic(err)
		}

		if g.PeerRecord, err = m.Mint(&g); err != nil {
			panic(err)
		}

		return append(gs, &g)
	}
}

type gossipSlice []*GossipRecord

func (gs gossipSlice) Len() int           { return len(gs) }
func (gs gossipSlice) Less(i, j int) bool { return gs[i].Hop() < gs[j].Hop() }
func (gs gossipSlice) Swap(i, j int)      { gs[i], gs[j] = gs[j], gs[i] }

// Validate a View that was received during a gossip round.
func (gs gossipSlice) Validate() error {
	if len(gs) == 0 {
		return ValidationError{Message: "empty view"}
	}

	// Non-senders should have a hop > 0
	for _, g := range gs[:len(gs)-1] {
		if g.Hop() == 0 {
			return ValidationError{
				Message: fmt.Sprintf("peer %s", g.PeerID.ShortString()),
				Cause:   fmt.Errorf("%w: expected hop > 0", ErrInvalidRange),
			}
		}
	}

	// Validate sender hop == 0
	if g := gs.last(); g.Hop() != 0 {
		return ValidationError{
			Message: fmt.Sprintf("sender %s", g.PeerID.ShortString()),
			Cause:   fmt.Errorf("%w: nonzero hop for sender", ErrInvalidRange),
		}
	}

	return nil
}

func (gs gossipSlice) Bind(f func(gossipSlice) gossipSlice) gossipSlice { return f(gs) }

func (gs gossipSlice) find(g *GossipRecord) (have *GossipRecord, found bool) {
	seek := g.PeerID
	for _, have = range gs {
		if found = seek == have.PeerID; found {
			break
		}
	}

	return
}

// n.b.:  panics if gs is empty.
func (gs gossipSlice) last() *GossipRecord { return gs[len(gs)-1] }

func (gs gossipSlice) incrHops() {
	for _, g := range gs {
		g.IncrHop()
	}
}

func (gs gossipSlice) diff(other gossipSlice) (diff gossipSlice) {
	for _, g := range gs {
		if _, found := other.find(g); !found {
			diff = append(diff, g)
		}
	}
	return
}

// convert last 8 bytes of a peer.ID into a unit64.
func lastUint64(s string) (u uint64) {
	for i := 0; i < 8; i++ {
		u = (u << 8) | uint64(s[len(s)-i-1])
	}

	return
}

func min(n1, n2 int) int {
	if n1 <= n2 {
		return n1
	}
	return n2
}

func max(n1, n2 int) int {
	if n1 <= n2 {
		return n2
	}
	return n1
}
