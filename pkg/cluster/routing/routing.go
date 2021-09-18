package routing

import (
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/lthibault/treap"
)

/*
 * model.go specifies a cluster model with PA/EL guarantees.
 */

var handle = treap.Handle{
	CompareKeys:    unsafePeerIDComparator,
	CompareWeights: unsafeTimeComparator,
}

type Record interface {
	Peer() peer.ID
	TTL() time.Duration
	Seq() uint64
}

type Iterator struct{ it *treap.Iterator }

func (i *Iterator) Next() (more bool)   { return i.it.Next() }
func (i *Iterator) More() bool          { return i.it.More() }
func (i *Iterator) Record() Record      { return i.it.Value.(Record) }
func (i *Iterator) Deadline() time.Time { return timeCastUnsafe(i.it.Weight) }

type state struct {
	T time.Time
	N *treap.Node
}

type Table atomic.Value

func New() *Table {
	v := new(atomic.Value)
	v.Store(state{T: time.Now()})
	return (*Table)(v)
}

func (tb *Table) Iter() Iterator { return Iterator{handle.Iter(tb.load().N)} }

func (tb *Table) Lookup(id peer.ID) (rec Record, ok bool) {
	var v interface{}
	if v, ok = handle.Get(tb.load().N, id); ok {
		rec, ok = v.(Record)
	}

	return
}

// Advance the state of the routing table to the current time.
// Expired entries will be evicted from the table.
func (tb *Table) Advance(t time.Time) {
	var old, new state
	for /* CAS loop */ {

		// is there any data?
		if old = tb.load(); old.N != nil {
			// evict stale entries
			for new = old; expired(t, new); {
				new = merge(t, new)
			}
		} else {
			// just update the time
			new = state{T: t, N: old.N}
		}

		if tb.compareAndSwap(old, new) {
			break
		}
	}
}

// Upsert inserts a record in the routing table, updating it
// if it already exists.
func (tb *Table) Upsert(rec Record) bool {
	var ok, created bool

	for {
		old := tb.load()
		new := old

		// upsert if seq is greater than the value stored in the treap -- non-blocking.
		new.N, created = handle.UpsertIf(new.N, rec.Peer(), rec, old.T.Add(rec.TTL()), func(n *treap.Node) bool {
			ok = newer(n, rec) // set return value for outer closure
			return ok
		})

		if tb.compareAndSwap(old, new) { // atomic
			break
		}
	}

	// The message should be processed iff the incoming message's sequence number is
	// greater than the one in the treap (ok==true) OR the id was just inserted into the
	// treap (created==true).
	return ok || created
}

func (tb *Table) load() state {
	v := (*atomic.Value)(tb).Load()
	return *(*state)((*ifaceWords)(unsafe.Pointer(&v)).data)
}

func (tb *Table) compareAndSwap(old, new state) bool {
	return (*atomic.Value)(tb).CompareAndSwap(old, new)
}

func merge(t time.Time, s state) state {
	return state{
		T: t,
		N: handle.Merge(s.N.Left, s.N.Right),
	}
}

func expired(t time.Time, s state) (ok bool) {
	if s.N != nil {
		ok = handle.CompareWeights(s.N.Weight, t) <= 0
	}

	return
}

func newer(n *treap.Node, r Record) bool {
	return n.Value.(Record).Seq() < r.Seq()
}
