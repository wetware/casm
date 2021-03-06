package pex

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"time"

	"capnproto.org/go/capnp/v3"
	ds "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/peer"
)

func init() { rand.Seed(time.Now().UnixNano()) }

type EvtViewUpdated []*GossipRecord

// namespace encapsulates a namespace-scoped store
// containing gossip records, along with supplementary
// data types from *PeerExchange needed to perform various
// queries.
//
// Note that namespace DOES NOT provide ACID guarantees.
type namespace struct {
	prefix ds.Key
	ds     ds.Batching
	id     peer.ID
	k      int
	e      event.Emitter
}

func (n namespace) String() string { return n.prefix.BaseNamespace() }

func (n namespace) Query() (query.Results, error) {
	return n.ds.Query(query.Query{
		Prefix: n.prefix.String(),
		Orders: []query.Order{randomOrder()},
	})
}

func (n namespace) Records() (gossipSlice, error) {
	// return all entries under the local instance's key prefix
	res, err := n.Query()
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

// View is like Records() except that it reuses a single GossipRecord
// to avoid allocating.
func (n namespace) View() ([]peer.AddrInfo, error) {
	// return all entries under the local instance's key prefix
	res, err := n.Query()
	if err != nil {
		return nil, err
	}

	es, err := res.Rest()
	if err != nil {
		return nil, err
	}

	var (
		g    GossipRecord
		view = make([]peer.AddrInfo, len(es))
	)

	for i, entry := range es {
		msg, err := capnp.Unmarshal(entry.Value)
		if err != nil {
			return nil, err
		}

		if err = g.ReadMessage(msg); err != nil {
			return nil, err
		}

		view[i].ID = g.PeerID
		view[i].Addrs = g.Addrs

		g.Message().Reset(nil)
	}

	return view, nil
}

func (n namespace) MergeAndStore(remote gossipSlice) error {
	if err := remote.Validate(); err != nil {
		return err
	}

	remote.incrHops()
	sender := remote.last()

	local, err := n.Records()
	if err != nil {
		return err
	}

	merged := remote.
		Bind(isNot(n.id)).
		Bind(merged(local)).
		Bind(ordered(n.id, sender)).
		Bind(tail(n.k))

	if err = n.Store(local, merged); err != nil {
		return err
	}

	if err = n.ds.Sync(n.prefix); err == nil {
		err = n.e.Emit(EvtViewUpdated(merged))
	}

	return err
}

func (n namespace) Store(old, new gossipSlice) error {
	batch, err := n.ds.Batch()
	if err != nil {
		return err
	}

	for _, g := range old.diff(new) { // elems in 'old', but not in 'new'.
		if err = batch.Delete(n.keyfor(g)); err != nil {
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

		if err = batch.Put(n.keyfor(g), b); err != nil {
			return err
		}
	}

	return batch.Commit()
}

func (n namespace) keyfor(g *GossipRecord) ds.Key {
	return n.prefix.ChildString(g.PeerID.String())
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
		filtered := gs[:0]
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
			gs.Bind(dedupe(tail)),
			tail.Bind(dedupe(gs))...)
	}
}

func dedupe(other gossipSlice) func(gossipSlice) gossipSlice {
	return func(gs gossipSlice) gossipSlice {
		return gs.Bind(filter(func(g *GossipRecord) bool {
			have, found := other.find(g)
			// select if:
			// unique   ...    more recent   ...  less diffused
			return !found || g.Seq > have.Seq || g.Hop() < have.Hop()
		}))
	}
}

func ordered(id peer.ID, sender *GossipRecord) func(gossipSlice) gossipSlice {
	const thresh = math.MaxUint64 / 2

	return func(gs gossipSlice) gossipSlice {
		if sender.Distance(id)/math.MaxUint64 > thresh {
			return gs.Bind(shuffled())
		}

		return gs.Bind(sorted())
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

func tail(n int) func(gossipSlice) gossipSlice {
	if n <= 0 {
		panic("n must be greater than zero.")
	}

	return func(gs gossipSlice) gossipSlice {
		if n < len(gs) {
			gs = gs[len(gs)-n:]
		}

		return gs
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
