package pex

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"capnproto.org/go/capnp/v3"
	ds "github.com/ipfs/go-datastore"
	nsds "github.com/ipfs/go-datastore/namespace"
	"github.com/ipfs/go-datastore/query"
	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/record"
	"github.com/lthibault/log"
	"go.uber.org/fx"
)

func init() { rand.Seed(time.Now().UnixNano()) }

type gossipStore struct {
	log      log.Logger
	localRec atomic.Value

	local   peer.ID
	MaxSize int

	mu sync.RWMutex
	ds ds.Batching

	e event.Emitter
}

type storeParams struct {
	fx.In

	NS      string
	Log     log.Logger
	ID      peer.ID
	Bus     event.Bus
	Store   ds.Batching
	MaxSize int
	Emitter event.Emitter
}

func newGossipStore(p storeParams, lx fx.Lifecycle) *gossipStore {
	prefix := ds.NewKey("/casm/pex").
		ChildString(p.NS).
		ChildString(p.ID.String())

	return &gossipStore{
		MaxSize: p.MaxSize,
		log:     p.Log.WithField("max_size", p.MaxSize),
		local:   p.ID,
		ds:      nsds.Wrap(p.Store, prefix),
		e:       p.Emitter,
	}
}

func (s *gossipStore) keyFor(g *GossipRecord) ds.Key {
	return ds.NewKey(g.PeerID.String())
}

func (s *gossipStore) Load() ([]*GossipRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.unsafeLoad()
}

func (s *gossipStore) MergeAndStore(remote view) error {
	if err := remote.Validate(); err != nil {
		return err
	}

	remote.incrHops()
	sender := remote.last()

	s.mu.Lock()
	defer s.mu.Unlock()

	local, err := s.unsafeLoad()
	if err != nil {
		return err
	}

	merged := selector(remote).
		Bind(isNot(s.local)).
		Bind(merged(local)).
		Bind(ordered(s.local, sender)).
		Bind(tail(s.MaxSize)).
		AsView()

	if err = s.unsafeStore(local, merged); err != nil {
		return err
	}

	// ensure persistence
	if err = s.ds.Sync(ds.NewKey("")); err == nil {
		err = s.e.Emit(EvtViewUpdated(merged))
	}

	return err
}

func (s *gossipStore) unsafeStore(old, new view) error {
	batch, err := s.ds.Batch()
	if err != nil {
		return err
	}

	for _, g := range old.diff(new) { // elems in 'old', but not in 'new'.
		if err = batch.Delete(s.keyFor(g)); err != nil {
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

		if err = batch.Put(s.keyFor(g), b); err != nil {
			return err
		}
	}

	return batch.Commit()
}

func (s *gossipStore) unsafeLoad() ([]*GossipRecord, error) {
	// return all entries under the local instance's key prefix
	res, err := s.ds.Query(query.Query{})
	if err != nil {
		return nil, err
	}

	es, err := res.Rest()
	if err != nil {
		return nil, err
	}

	view := make([]*GossipRecord, len(es))
	for i, entry := range es {
		view[i] = new(GossipRecord) // TODO:  pool?

		msg, err := capnp.Unmarshal(entry.Value)
		if err != nil {
			return nil, err
		}

		if err = view[i].ReadMessage(msg); err != nil {
			return nil, err
		}
	}

	return view, nil
}

func (s *gossipStore) SetLocalRecord(e *record.Envelope) error {
	g, err := NewGossipRecord(e)
	if err == nil {
		s.localRec.Store(g)
	}
	return err
}

func (s *gossipStore) GetLocalRecord() *GossipRecord { return s.localRec.Load().(*GossipRecord) }

type selector view

func (sel selector) AsView() view { return view(sel) }

func (sel selector) Bind(f func(view) selector) selector {
	return f(view(sel))
}

func filter(f func(*GossipRecord) bool) func(view) selector {
	return func(v view) selector {
		filtered := selector(v[:0])
		for _, g := range v {
			if f(g) {
				filtered = append(filtered, g)
			}
		}
		return filtered
	}
}

func isNot(self peer.ID) func(view) selector {
	return filter(func(g *GossipRecord) bool {
		return self != g.PeerID
	})
}

func merged(tail view) func(view) selector {
	return func(v view) selector {
		return append(
			selector(v).Bind(dedupe(tail)),
			selector(tail).Bind(dedupe(v))...)
	}
}

func dedupe(other view) func(view) selector {
	return func(v view) selector {
		return selector(v).Bind(filter(func(g *GossipRecord) bool {
			have, found := other.find(g)
			// select if:
			// unique   ...    more recent   ...  less diffused
			return !found || g.Seq > have.Seq || g.Hop() < have.Hop()
		}))
	}
}

func ordered(id peer.ID, sender *GossipRecord) func(view) selector {
	const thresh = math.MaxUint64 / 2

	return func(v view) selector {
		if sender.Distance(id)/math.MaxUint64 > thresh {
			return selector(v).Bind(shuffled())
		}

		return selector(v).Bind(sorted())
	}
}

func shuffled() func(view) selector {
	return func(v view) selector {
		rand.Shuffle(len(v), v.Swap)
		return selector(v)
	}
}

func sorted() func(view) selector {
	return func(v view) selector {
		sort.Sort(v)
		return selector(v)
	}
}

func tail(n int) func(view) selector {
	if n <= 0 {
		panic("n must be greater than zero.")
	}

	return func(v view) selector {
		if n < len(v) {
			v = v[len(v)-n:]
		}

		return selector(v)
	}
}

type view []*GossipRecord

func (v view) Len() int           { return len(v) }
func (v view) Less(i, j int) bool { return v[i].Hop() < v[j].Hop() }
func (v view) Swap(i, j int)      { v[i], v[j] = v[j], v[i] }

// Validate a View that was received during a gossip round.
func (v view) Validate() error {
	if len(v) == 0 {
		return ValidationError{Message: "empty view"}
	}

	// Non-senders should have a hop > 0
	for _, g := range v[:len(v)-1] {
		if g.Hop() == 0 {
			return ValidationError{
				Message: fmt.Sprintf("peer %s", g.PeerID.ShortString()),
				Cause:   fmt.Errorf("%w: expected hop > 0", ErrInvalidRange),
			}
		}
	}

	// Validate sender hop == 0
	if g := v.last(); g.Hop() != 0 {
		return ValidationError{
			Message: fmt.Sprintf("sender %s", g.PeerID.ShortString()),
			Cause:   fmt.Errorf("%w: nonzero hop for sender", ErrInvalidRange),
		}
	}

	return nil
}

func (v view) find(g *GossipRecord) (have *GossipRecord, found bool) {
	seek := g.PeerID
	for _, have = range v {
		if found = seek == have.PeerID; found {
			break
		}
	}

	return
}

// n.b.:  panics if v is empty.
func (v view) last() *GossipRecord { return v[len(v)-1] }

func (v view) incrHops() {
	for _, g := range v {
		g.IncrHop()
	}
}

func (v view) diff(other view) (diff view) {
	for _, g := range v {
		if _, found := other.find(g); !found {
			diff = append(diff, g)
		}
	}
	return
}

// convert last 8 bytes of a peer.ID into a unit64.
func lastUint64(id peer.ID) (u uint64) {
	for i := 0; i < 8; i++ {
		u = (u << 8) | uint64(id[len(id)-i-1])
	}

	return
}
