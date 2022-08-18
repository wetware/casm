package routing

import (
	"github.com/hashicorp/go-memdb"
	"github.com/wetware/casm/pkg/stm"
)

// type Index struct{ routing.View_Index }

// func (ix Index) String() string {
// 	switch ix.Which() {
// 	case routing.View_Index_Which_peer:
// 		return "id"

// 	case routing.View_Index_Which_peerPrefix:
// 		return "id_prefix"

// 	case routing.View_Index_Which_hostPrefix:
// 		return "host_prefix"

// 	case routing.View_Index_Which_metaPrefix:
// 		return "meta_prefix"

// 	default:
// 		return ix.Which().String()
// 	}
// }

// func (ix Index) Match(r Record) bool {
// 	switch ix.Which() {
// 	case routing.View_Index_Which_peer:
// 		id, err := ix.Peer()
// 		return err == nil && id == string(r.Peer())

// 	case routing.View_Index_Which_peerPrefix:
// 		id, err := ix.PeerPrefix()
// 		return err == nil && strings.HasPrefix(string(r.Peer()), id)

// 	case routing.View_Index_Which_host:
// 		index, err1 := ix.Host()
// 		name, err2 := r.Host()
// 		return err1 == nil && err2 == nil && name == index

// 	case routing.View_Index_Which_hostPrefix:
// 		prefix, err1 := ix.HostPrefix()
// 		name, err2 := r.Host()
// 		return err1 == nil && err2 == nil &&
// 			strings.HasPrefix(name, prefix)

// 	case routing.View_Index_Which_meta:
// 		index, err1 := ix.Meta()
// 		meta, err2 := r.Meta()
// 		return err1 == nil && err2 == nil &&
// 			matchMeta(fieldEq, meta, Meta(index))

// 	case routing.View_Index_Which_metaPrefix:
// 		index, err1 := ix.MetaPrefix()
// 		meta, err2 := r.Meta()
// 		return err1 == nil && err2 == nil &&
// 			matchMeta(strings.HasPrefix, meta, Meta(index))
// 	}

// 	return false
// }

// func matchMeta(match func(ix, m string) bool, meta, index Meta) bool {
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

type query struct {
	records stm.TableRef
	tx      stm.Txn
}

func (q query) Get(ix Index) (Iterator, error) {
	return q.iterate(q.tx.Get, ix)
}

func (q query) GetReverse(ix Index) (Iterator, error) {
	return q.iterate(q.tx.GetReverse, ix)
}

func (q query) LowerBound(ix Index) (Iterator, error) {
	return q.iterate(q.tx.LowerBound, ix)
}

func (q query) ReverseLowerBound(ix Index) (Iterator, error) {
	return q.iterate(q.tx.ReverseLowerBound, ix)
}

func (q query) iterate(f iterFunc, ix Index) (Iterator, error) {
	it, err := f(q.records, ix.String(), ix)
	return iterator{it}, err
}

type iterFunc func(stm.TableRef, string, ...any) (memdb.ResultIterator, error)

type iterator struct{ memdb.ResultIterator }

func (it iterator) Next() Record {
	if v := it.ResultIterator.Next(); v != nil {
		return v.(Record)
	}

	return nil
}
