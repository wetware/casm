package routing

import (
	"errors"
	"strings"
	"sync/atomic"
	"time"

	"capnproto.org/go/capnp/v3"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/lthibault/treap"
)

var handle = treap.Handle{
	CompareKeys:    pidComparator,
	CompareWeights: treap.TimeComparator,
}

type Record interface {
	Peer() peer.ID
	TTL() time.Duration
	Seq() uint64
}

// Iterator is a stateful object that enumerates routing
// records.  Iterator's methods are NOT guaranteed to be
// thread-safe, but implementations MUST permit multiple
// iterators to operate concurently.
//
// Iterators are snapshots of the routing table and SHOULD
// be consumed quickly.
type Iterator interface {
	// Next updates the iterator's internal state, causing
	// Record() and Deadline() to return different values.
	// This invalidates any previous Records produced by a
	// call to Record().
	Next()

	// Record returns the current routing record.  If nil
	// is returned, the iterator is exhausted. All method
	// calls on an exhausted iterator, other than Finish,
	// are undefined.
	//
	// A value returned by a call to Record is valid until
	// the next call to Next or Finish.
	Record() Record

	// Deadline returns the local time at which the record
	// will be removed from the routing table.   Note that
	// deadlines are approximate, and that their precision
	// MAY vary across implementations.
	Deadline() time.Time

	// Finish releases any resources owned by the iterator,
	// invalidating any values returned by Record().
	//
	// Callers MUST call Finish before discarding an iterator
	// that has not been exhausted via calls to Next().
	Finish()
}

type Meta capnp.TextList

func (m Meta) Len() int {
	return capnp.TextList(m).Len()
}

func (m Meta) Get(key string) (string, error) {
	for i := 0; i < m.Len(); i++ {
		s, err := capnp.TextList(m).At(i)
		if err != nil {
			return "", err
		}

		if strings.HasPrefix(s, key) {
			field, err := parseField(s)
			if err != nil {
				return "", err
			}

			return field.Value(), nil

		}
	}

	return "", nil
}

func (m Meta) Index() (indexes [][]byte, err error) {
	indexes = make([][]byte, m.Len())
	for i := range indexes {
		if indexes[i], err = capnp.TextList(m).BytesAt(i); err != nil {
			break
		}
	}

	return
}

type field struct {
	s   string
	idx int
}

func parseField(s string) (field, error) {
	idx := strings.Index(s, "=")
	if idx < 0 {
		return field{}, errors.New("separator not found")
	}

	if idx == 0 {
		return field{}, errors.New("missing key")
	}

	return field{s: s, idx: idx}, nil
}

func (s field) Key() string {
	return s.s[:s.idx]
}

func (s field) Value() string {
	return s.s[s.idx+1:]
}

type treapIter struct{ *treap.Iterator }

func (i treapIter) Deadline() time.Time { return i.Weight.(time.Time) }

func (i treapIter) Record() Record {
	if i.Node == nil {
		return nil
	}

	return i.Value.(Record)
}

type state struct {
	T time.Time
	N *treap.Node
}

// Table is a fast, in-memory routing table.
type Table atomic.Value

func New() *Table {
	v := new(atomic.Value)
	v.Store(state{})
	return (*Table)(v)
}

func (tb *Table) Iter() Iterator { return treapIter{handle.Iter(tb.load().N)} }

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
	for {
		if old := tb.load(); tb.compareAndSwap(old, evicted(old, t)) {
			break
		}
	}
}

// Upsert inserts a record in the routing table, updating it
// if it already exists.
func (tb *Table) Upsert(rec Record) bool {
	var ok, created bool

	for {
		ok = false
		created = false

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
	return (*atomic.Value)(tb).Load().(state)
}

func (tb *Table) compareAndSwap(old, new state) bool {
	return (*atomic.Value)(tb).CompareAndSwap(old, new)
}

func evicted(s state, t time.Time) state {
	s.T = t
	for expired(s, t) {
		s.N = handle.Merge(s.N.Left, s.N.Right)
	}

	return s
}

func expired(s state, t time.Time) bool {
	return s.N != nil && handle.CompareWeights(s.N.Weight, t) <= 0
}

func newer(n *treap.Node, r Record) bool {
	return r.Seq() > n.Value.(Record).Seq()
}
