package view

import (
	"strings"

	api "github.com/wetware/casm/internal/api/routing"
	"github.com/wetware/casm/pkg/cluster/routing"
)

type index struct{ api.View_Index }

func (ix index) String() string {
	switch ix.Which() {
	case api.View_Index_Which_peer:
		return "id"

	case api.View_Index_Which_peerPrefix:
		return "id_prefix"

	case api.View_Index_Which_hostPrefix:
		return "host_prefix"

	case api.View_Index_Which_metaPrefix:
		return "meta_prefix"

	default:
		return ix.Which().String()
	}
}

func (ix index) Match(r routing.Record) bool {
	switch ix.Which() {
	case api.View_Index_Which_peer:
		id, err := ix.Peer()
		return err == nil && id == string(r.Peer())

	case api.View_Index_Which_peerPrefix:
		id, err := ix.PeerPrefix()
		return err == nil && strings.HasPrefix(string(r.Peer()), id)

	case api.View_Index_Which_host:
		index, err1 := ix.Host()
		name, err2 := r.Host()
		return err1 == nil && err2 == nil && name == index

	case api.View_Index_Which_hostPrefix:
		prefix, err1 := ix.HostPrefix()
		name, err2 := r.Host()
		return err1 == nil && err2 == nil &&
			strings.HasPrefix(name, prefix)

	case api.View_Index_Which_meta:
		index, err1 := ix.Meta()
		meta, err2 := r.Meta()
		return err1 == nil && err2 == nil &&
			matchMeta(fieldEq, meta, routing.Meta(index))

	case api.View_Index_Which_metaPrefix:
		index, err1 := ix.MetaPrefix()
		meta, err2 := r.Meta()
		return err1 == nil && err2 == nil &&
			matchMeta(strings.HasPrefix, meta, routing.Meta(index))
	}

	return false
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
