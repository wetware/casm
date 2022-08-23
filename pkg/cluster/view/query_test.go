package view_test

import (
	"testing"

	"capnproto.org/go/capnp/v3"

	"github.com/stretchr/testify/assert"

	api "github.com/wetware/casm/internal/api/routing"
	"github.com/wetware/casm/pkg/cluster/routing"
	"github.com/wetware/casm/pkg/cluster/view"
)

func TestQuery(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name  string
		query view.Query
		which api.View_Selector_Which
		param queryParams
	}{
		{
			name:  "All",
			query: view.All(),
			which: api.View_Selector_Which_all,
		},
		{
			name:  "Select",
			query: view.Select(index(routing.HostKey, "foo")),
			which: api.View_Selector_Which_match,
		},
		{
			name:  "From",
			query: view.From(index(routing.HostKey, "foo")),
			which: api.View_Selector_Which_from,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tt.query(&tt.param)
			assert.Equal(t, tt.which, tt.param.S.Which(),
				"selector should be %s", tt.which)
		})

	}
}

type queryParams struct {
	S  api.View_Selector
	Cs api.View_Constraint_List
}

func (ps *queryParams) NewSelector() (api.View_Selector, error) {
	_, seg := capnp.NewSingleSegmentMessage(nil)
	s, err := api.NewRootView_Selector(seg)
	ps.S = s
	return s, err
}

func (ps *queryParams) NewConstraints(size int32) (api.View_Constraint_List, error) {
	_, seg := capnp.NewSingleSegmentMessage(nil)
	cs, err := api.NewView_Constraint_List(seg, size)
	ps.Cs = cs
	return cs, err
}

type mockIndex struct {
	IndexKey routing.IndexKey
	Value    string
}

func index(key routing.IndexKey, value string) mockIndex {
	return mockIndex{
		IndexKey: key,
		Value:    value,
	}
}

func (mockIndex) String() string            { return "test index" }
func (mockIndex) Match(routing.Record) bool { return false }
func (i mockIndex) Key() routing.IndexKey   { return i.IndexKey }
func (i mockIndex) Host() (string, error)   { return i.Value, nil }
func (i mockIndex) Peer() (string, error)   { return i.Value, nil }
