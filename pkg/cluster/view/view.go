package view

import (
	"fmt"

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
	return func(q routing.Query) (it routing.Iterator, err error) {
		if it, err = selection(q); err == nil {
			it, err = f(it)(q)
		}

		return
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

func Range(min, max routing.Index) Selector {
	return From(min).Bind(To(max))
}

func failure(err error) Selector {
	return func(q routing.Query) (routing.Iterator, error) {
		return nil, err
	}
}

func failuref(format string, args ...any) Selector {
	return failure(fmt.Errorf(format, args...))
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

func While(predicate Matcher) Constraint {
	return func(it routing.Iterator) Selector {
		return just(&predicateIter{
			Matcher:  predicate,
			Iterator: it,
		})
	}
}

func Limit(n int) Constraint {
	if n <= 0 {
		return func(routing.Iterator) Selector {
			return failuref("expected limit > 0 (got %d)", n)
		}
	}

	return While(matchFunc(func(r routing.Record) (ok bool) {
		ok = n > 0
		n--
		return
	}))
}

func To(ix routing.Index) Constraint {
	return While(leq(ix))
}

func First() Constraint {
	return Limit(1)
}

func just(it routing.Iterator) Selector {
	return func(routing.Query) (routing.Iterator, error) {
		return it, nil
	}
}

func leq(m Matcher) matchFunc {
	var reached bool
	return func(r routing.Record) bool {
		match := m.Match(r)
		reached = reached || match
		return !reached || match
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
	for r = it.Iterator.Next(); r != nil && !it.Match(r); r = it.Iterator.Next() {
	}

	return
}

// predicateIter is short-circuits when the Matcher returns false.
// This is more efficient than using filterIter in cases where the
// iterator should stop early.
type predicateIter struct {
	Matcher
	routing.Iterator
	stop bool
}

func (it *predicateIter) Next() (r routing.Record) {
	if !it.stop {
		r = it.Iterator.Next()
		if it.stop = !it.Match(r); it.stop {
			r = nil
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
