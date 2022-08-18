package view

import (
	"github.com/wetware/casm/pkg/cluster/routing"
)

type Matcher interface {
	Match(routing.Record) bool
}

type View struct {
	routing.Query
}

// Lookup returns the the first(lowest key) Record matching the selector
// and constraints.  Record is nil if there is no match.
func (v View) Lookup(s Selector, cs ...Constraint) (routing.Record, error) {
	it, err := v.Iter(s, cs...)
	if it == nil || err != nil {
		return nil, err
	}

	return it.Next(), nil
}

// Iter returns an iterator ranging over the Records that match the selector
// and constraints. The iterator's Next() method returns nil if there are no
// matches.
func (v View) Iter(s Selector, cs ...Constraint) (routing.Iterator, error) {
	for _, constraint := range cs {
		s = s.Bind(constraint)
	}

	return s(v.Query)
}

// Reverse returns a View that iterates in reverse key-order.
func (v View) Reverse() View {
	if r, ok := v.Query.(reversed); ok {
		return View(r)
	}

	return View{Query: reversed(v)}
}

// Selector specifies a set of records.
type Selector func(routing.Query) (routing.Iterator, error)

func (selection Selector) Bind(f Constraint) Selector {
	return func(q routing.Query) (routing.Iterator, error) {
		it, err := selection(q)
		if err != nil {
			return nil, err
		}

		return f(it)(q)
	}
}

func All() Selector {
	return Match(all{})
}

func Match(ix routing.Index) Selector {
	return func(q routing.Query) (routing.Iterator, error) {
		return q.Get(ix)
	}
}

func From(ix routing.Index) Selector {
	return func(q routing.Query) (routing.Iterator, error) {
		return q.LowerBound(ix)
	}
}

type Constraint func(routing.Iterator) Selector

func Where(match Matcher) Constraint {
	return func(it routing.Iterator) Selector {
		return just(&filterIter{
			Matcher:  match,
			Iterator: it,
		})
	}
}

func Limit(n int) Constraint {
	return Where(limit(n))
}

func To(ix routing.Index) Constraint {
	return Where(boundary(ix))
}

func Until(ix routing.Index) Constraint {
	return Where(ix)
}

func just(it routing.Iterator) Selector {
	return func(routing.Query) (routing.Iterator, error) {
		return it, nil
	}
}

func limit(n int) matchFunc {
	return func(routing.Record) bool {
		n--
		return n >= 0
	}
}

func boundary(m Matcher) matchFunc {
	var reached bool
	return func(r routing.Record) bool {
		if reached = m.Match(r); reached {
			reached = true
		} else if reached {
			return false
		}

		return true
	}
}

type matchFunc func(routing.Record) bool

func (match matchFunc) Match(r routing.Record) bool {
	return match(r)
}

type filterIter struct {
	Matcher
	routing.Iterator
}

func (it *filterIter) Next() (r routing.Record) {
	for r = it.Iterator.Next(); r != nil; r = it.Iterator.Next() {
		if it.Match(r) {
			break
		}
	}

	return
}

type reversed struct{ routing.Query }

func (r reversed) Get(ix routing.Index) (routing.Iterator, error) {
	return r.Query.GetReverse(ix)
}

func (r reversed) GetReverse(ix routing.Index) (routing.Iterator, error) {
	return r.Query.Get(ix)
}

func (r reversed) LowerBound(ix routing.Index) (routing.Iterator, error) {
	return r.Query.ReverseLowerBound(ix)
}

func (r reversed) ReverseLowerBound(ix routing.Index) (routing.Iterator, error) {
	return r.Query.LowerBound(ix)
}

type all struct{}

func (all) String() string             { return "id" }
func (all) PeerBytes() ([]byte, error) { return nil, nil }
func (all) Match(routing.Record) bool  { return true }

// type index struct{ api.View_Index }

// func (ix index) String() string {
// 	switch ix.Which() {
// 	case api.View_Index_Which_peer:
// 		return "id"

// 	case api.View_Index_Which_peerPrefix:
// 		return "id_prefix"

// 	case api.View_Index_Which_hostPrefix:
// 		return "host_prefix"

// 	case api.View_Index_Which_metaPrefix:
// 		return "meta_prefix"

// 	default:
// 		return ix.Which().String()
// 	}
// }

// func (ix index) Match(r routing.Record) bool {
// 	switch ix.Which() {
// 	case api.View_Index_Which_peer:
// 		id, err := ix.Peer()
// 		return err == nil && id == string(r.Peer())

// 	case api.View_Index_Which_peerPrefix:
// 		id, err := ix.PeerPrefix()
// 		return err == nil && strings.HasPrefix(string(r.Peer()), id)

// 	case api.View_Index_Which_host:
// 		index, err1 := ix.Host()
// 		name, err2 := r.Host()
// 		return err1 == nil && err2 == nil && name == index

// 	case api.View_Index_Which_hostPrefix:
// 		prefix, err1 := ix.HostPrefix()
// 		name, err2 := r.Host()
// 		return err1 == nil && err2 == nil &&
// 			strings.HasPrefix(name, prefix)

// 	case api.View_Index_Which_meta:
// 		index, err1 := ix.Meta()
// 		meta, err2 := r.Meta()
// 		return err1 == nil && err2 == nil &&
// 			matchMeta(fieldEq, meta, routing.Meta(index))

// 	case api.View_Index_Which_metaPrefix:
// 		index, err1 := ix.MetaPrefix()
// 		meta, err2 := r.Meta()
// 		return err1 == nil && err2 == nil &&
// 			matchMeta(strings.HasPrefix, meta, routing.Meta(index))
// 	}

// 	return false
// }

// func matchMeta(match func(ix, m string) bool, meta, index routing.Meta) bool {
// 	if index.Len() == 0 {
// 		return meta.Len() == 0
// 	}

// 	for i := 0; i < index.Len(); i++ {
// 		f, err := index.At(i)
// 		if err != nil {
// 			return false
// 		}

// 		value, err := meta.Get(f.Key)
// 		if err != nil || !match(value, f.Value) {
// 			return false
// 		}
// 	}

// 	return true
// }

// func fieldEq(index, meta string) bool { return index == meta }

/*

	------------------------------------------------------------------------------------------------------------------------------

*/

// func (v View) Lookup(ctx context.Context, call api.View_lookup) error {
// 	s, err := call.Args().Selector()
// 	if err != nil {
// 		return err
// 	}

// 	return v.bind(s, record(call))

// }

// func (v View) Iter(ctx context.Context, call api.View_iter) error {
// 	s, err := call.Args().Selector()
// 	if err != nil {
// 		return err
// 	}

// 	return v.bind(s, iterator(ctx, call))
// }

// func (v View) bind(s api.View_Selector, rec allocator) error {
// 	switch s.Which() {
// 	case api.View_Selector_Which_match:
// 		return bind(rec, v.match(s))

// 	case api.View_Selector_Which_range:
// 		return bind(rec, v.matchRange(s))
// 	}

// 	return fmt.Errorf("invalid selector: %s", s.Which())
// }

// func (v View) match(s api.View_Selector) selector {
// 	return func() (routing.Iterator, error) {
// 		match, err := s.Match()
// 		if err != nil {
// 			return nil, err
// 		}

// 		return v.Get(index(match))
// 	}
// }

// func (v View) matchRange(s api.View_Selector) selector {
// 	return func() (routing.Iterator, error) {
// 		min, err := s.Range().Min()
// 		if err != nil {
// 			return nil, err
// 		}

// 		max, err := s.Range().Max()
// 		if err != nil {
// 			return nil, err
// 		}

// 		return v.newRangeIter(index(min), index(max))
// 	}
// }

// func (v View) newRangeIter(min, max routing.Index) (routing.Iterator, error) {
// 	if min.Which() != max.Which() {
// 		return nil, fmt.Errorf("invalid range: [%s, %s]", min, max)
// 	}

// 	it, err := v.LowerBound(min)
// 	if err != nil {
// 		return nil, err
// 	}

// 	return &rangeIter{
// 		limit:    max,
// 		Iterator: it,
// 	}, err
// }

// type selector func() (routing.Iterator, error)
// type allocator func(routing.Record) error

// func bind(alloc allocator, next selector) error {
// 	it, err := next()
// 	if err != nil {
// 		return err
// 	}

// 	for r := it.Next(); r != nil; r = it.Next() {
// 		if err = alloc(r); err != nil {
// 			break
// 		}
// 	}

// 	return err
// }

// func record(call api.View_lookup) allocator {
// 	return func(r routing.Record) error {
// 		res, err := call.AllocResults()
// 		if err != nil {
// 			return err
// 		}

// 		return maybe(res, r)
// 	}
// }

// func iterator(ctx context.Context, call api.View_iter) allocator {
// 	panic("NOT IMPLEMENTED")
// 	// res, err := call.AllocResults()
// 	// if err != nil {
// 	// 	return func(routing.Record) error { return err }
// 	// }

// 	// sender := call.Args().Handler()
// 	// sent := uint32(0) // TODO:  atomic?
// 	// return func(r routing.Record) error {
// 	// 	if sent < call.Args().Limit() {
// 	// 		f, release := sender.Send(ctx, )
// 	// 	}

// 	// 	return nil
// 	// }
// }

// func maybe(res api.View_lookup_Results, r routing.Record) error {
// 	if r == nil {
// 		return nil
// 	}

// 	result, err := res.NewResult()
// 	if err != nil {
// 		return err
// 	}

// 	rec, err := result.NewJust()
// 	if err != nil {
// 		return err
// 	}

// 	return copyRecord(rec, r)
// }

// func copyRecord(rec api.View_Record, r routing.Record) error {
// 	if b, ok := r.(RecordBinder); ok {
// 		return b.Bind(rec)
// 	}

// 	if err := rec.SetPeer(string(r.Peer())); err != nil {
// 		return err
// 	}

// 	hb, err := rec.NewHeartbeat()
// 	if err != nil {
// 		return err
// 	}

// 	pulse.Heartbeat{Heartbeat: hb}.SetTTL(r.TTL())
// 	hb.SetInstance(r.Instance())

// 	if err := copyHost(hb, r); err != nil {
// 		return err
// 	}

// 	return copyMeta(hb, r)
// }

// func copyHost(rec api.Heartbeat, r routing.Record) error {
// 	name, err := r.Host()
// 	if err == nil {
// 		err = rec.SetHost(name)
// 	}

// 	return err
// }

// func copyMeta(rec api.Heartbeat, r routing.Record) error {
// 	meta, err := r.Meta()
// 	if err == nil {
// 		err = rec.SetMeta(capnp.TextList(meta))
// 	}

// 	return err
// }

// type rangeIter struct {
// 	limit routing.Index
// 	routing.Iterator
// }

// func (it rangeIter) Next() routing.Record {
// 	r := it.Iterator.Next()
// 	if r == nil || it.limit.Match(r) {
// 		return nil
// 	}

// 	return r
// }

// func index(ix api.View_Index) routing.Index {
// 	return routing.Index{View_Index: ix}
// }
