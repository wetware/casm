package query

import (
	"github.com/wetware/casm/pkg/cluster/routing"
)

// Matcher reports whether a routing record matches some
// arbitrary criteria.
type Matcher interface {
	// Match returns true if the supplied record matches
	// some arbitrary criteria.  It is used to implement
	// filtering.  See:  Where.
	Match(routing.Record) bool
}

// Constraints restrict the results of a selector to a subset
// of current matches.
type Constraint func(routing.Iterator) Selector

// Where restricts a selection according to some arbitrary
// criteria.  It is effectively a filter.
func Where(match Matcher) Constraint {
	return func(it routing.Iterator) Selector {
		return just(&filterIter{
			Matcher:  match,
			Iterator: it,
		})
	}
}

// While iterates over a selection until the predicate returns
// boolean false.
func While(predicate Matcher) Constraint {
	return func(it routing.Iterator) Selector {
		return just(&predicateIter{
			Matcher:  predicate,
			Iterator: it,
		})
	}
}

// Limit restricts the selection to n items.
func Limit(n int) Constraint {
	if n <= 0 {
		return func(routing.Iterator) Selector {
			return Failuref("expected limit > 0 (got %d)", n)
		}
	}

	return While(matchFunc(func(r routing.Record) (ok bool) {
		ok = n > 0
		n--
		return
	}))
}

// To restricts the selection to items less-than-or-equal-to
// the index.  Use with From to implement range queries.
func To(index routing.Index) Constraint {
	return While(leq(index))
}

// First restricts the selection to a single item.
func First() Constraint {
	return Limit(1)
}

func just(it routing.Iterator) Selector {
	return func(routing.Snapshot) (routing.Iterator, error) {
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
