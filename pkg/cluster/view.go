//go:generate mockgen -source=view.go -destination=../../../internal/mock/pkg/cluster/view/view.go -package=mock_view

package cluster

import (
	"fmt"

	api "github.com/wetware/casm/internal/api/routing"
	"github.com/wetware/casm/pkg/cluster/routing"
)

type (
	Selector   func(api.View_Selector) error
	Constraint func(api.View_Constraint) error
)

type QueryParams interface {
	NewSelector() (api.View_Selector, error)
	NewConstraints(int32) (api.View_Constraint_List, error)
}

type Query func(QueryParams) error

func NewQuery(s Selector, cs ...Constraint) Query {
	return func(ps QueryParams) error {
		if err := bindSelector(s, ps); err != nil {
			return err
		}

		return bindConstraints(cs, ps)
	}
}

func bindSelector(s Selector, ps QueryParams) error {
	sel, err := ps.NewSelector()
	if err != nil {
		return err
	}

	return s(sel)
}

func bindConstraints(cs []Constraint, ps QueryParams) error {
	constraint, err := ps.NewConstraints(int32(len(cs)))
	if err != nil {
		return err
	}

	for i, bind := range cs {
		if err = bind(constraint.At(i)); err != nil {
			break
		}
	}

	return err
}

type index struct{ api.View_Index }

func (ix index) String() string {
	var (
		key = ix.Key()
		val string
	)

	switch key {
	case routing.PeerKey:
		val, _ = ix.Peer()
	case routing.PeerPrefixKey:
		val, _ = ix.PeerPrefix()
	case routing.HostKey:
		val, _ = ix.Host()
	case routing.HostPrefixKey:
		val, _ = ix.HostPrefix()
	case routing.MetaKey:
		meta, _ := ix.Meta()
		val = routing.Meta(meta).String()
	case routing.MetaPrefixKey:
		meta, _ := ix.MetaPrefix()
		val = routing.Meta(meta).String()
	default:
		return ix.String()
	}

	return fmt.Sprintf("%s=%s", ix.Key(), val)
}

func (ix index) Key() routing.IndexKey {
	return routing.IndexKey(ix.Which())
}
