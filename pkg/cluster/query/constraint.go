package query

import (
	"fmt"
	"strings"

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
	matcher, err := matchIndex(index)
	if err != nil {
		return failure(err)
	}

	return While(leq(matcher))
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

func failure(err error) Constraint {
	return func(routing.Iterator) Selector {
		return Failure(err)
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

func matchIndex(ix routing.Index) (matchFunc, error) {
	switch ix.Key() {
	case routing.PeerKey:
		id, err := ix.(interface{ Peer() (string, error) }).Peer()
		return func(r routing.Record) bool {
			return id == string(r.Peer())
		}, err

	case routing.PeerPrefixKey:
		id, err := ix.(interface{ PeerPrefix() (string, error) }).PeerPrefix()

		return func(r routing.Record) bool {
			return strings.HasPrefix(string(r.Peer()), id)
		}, err

	case routing.HostKey:
		index, err := ix.(interface{ Host() (string, error) }).Host()
		return func(r routing.Record) bool {
			name, err := r.Host()
			return err == nil && name == index
		}, err

	case routing.HostPrefixKey:
		prefix, err := ix.(interface{ HostPrefix() (string, error) }).HostPrefix()
		return func(r routing.Record) bool {
			name, err := r.Host()
			return err == nil && strings.HasPrefix(name, prefix)
		}, err

	case routing.MetaKey:
		index, err := ix.(interface{ Meta() (routing.Meta, error) }).Meta()
		return func(r routing.Record) bool {
			meta, err := r.Meta()
			return err == nil && matchMeta(fieldEq, meta, routing.Meta(index))
		}, err

	case routing.MetaPrefixKey:
		index, err := ix.(interface{ MetaPrefix() (routing.Meta, error) }).MetaPrefix()
		return func(r routing.Record) bool {
			meta, err := r.Meta()
			return err == nil && matchMeta(strings.HasPrefix, meta, routing.Meta(index))
		}, err
	}

	return nil, fmt.Errorf("invalid index: %s", ix)
}

type matchFunc func(routing.Record) bool

func (match matchFunc) Match(r routing.Record) bool {
	return match(r)
}

func matchMeta(match func(ix, m string) bool, meta, index routing.Meta) bool {
	if index.Len() == 0 {
		return meta.Len() == 0
	}

	for i := 0; i < index.Len(); i++ {
		f, err := index.At(i)
		if err != nil {
			return false
		}

		value, err := meta.Get(f.Key)
		if err != nil || !match(value, f.Value) {
			return false
		}
	}

	return true
}

func fieldEq(index, meta string) bool { return index == meta }
