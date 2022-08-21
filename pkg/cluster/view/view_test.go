package view_test

import (
	"math/rand"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mock_routing "github.com/wetware/casm/internal/mock/pkg/cluster/routing"
	"github.com/wetware/casm/pkg/cluster/routing"
	"github.com/wetware/casm/pkg/cluster/view"
)

func TestView(t *testing.T) {
	t.Parallel()
	t.Helper()

	recs := []*record{
		{id: newPeerID()},
		{id: newPeerID()},
		{id: newPeerID()},
		{id: newPeerID()},
		{id: newPeerID()},
	}

	t.Run("Lookup", func(t *testing.T) {
		t.Parallel()
		t.Helper()

		t.Run("Forward", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			/*
				For the forward lookup test, we'll actually check that
				the method call returns the expected result.   For the
				reverse lookup test, we'll take a simplified approach.
			*/

			iter := mock_routing.NewMockIterator(ctrl)
			iter.EXPECT().
				Next().
				Return(recs[0]).
				Times(1)

			query := mock_routing.NewMockQuery(ctrl)
			query.EXPECT().
				Get(gomock.Any()).
				Return(iter, nil).
				Times(1)

			// Double-reverse so that we test the code path that
			// takes us *back* to a normal, forward-iterating view.
			v := view.View{Query: query}.Reverse().Reverse()

			r, err := v.Lookup(view.All())
			require.NoError(t, err, "lookup should succeed")
			require.NotNil(t, r, "should return record")

			want := routing.Record(recs[0])
			assert.Equal(t, want, r, "should match %s", recs[0].Peer())
		})

		t.Run("Reverse", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			/*
				Just test that reversing the view calls the expected query method.
				In addition to simplifying test code, it explores the code path in
				which we abort if a nil iterator is returned the internal call to
				v.Iter()
			*/

			query := mock_routing.NewMockQuery(ctrl)
			query.EXPECT().
				GetReverse(gomock.Any()). // <- what we're actually testing
				Return(nil, nil).
				Times(1)

			v := view.View{Query: query}.Reverse()

			_, err := v.Lookup(view.All())
			require.NoError(t, err, "lookup should succeed")
		})
	})

	t.Run("Iter", func(t *testing.T) {
		t.Parallel()
		t.Helper()

		t.Run("Forward", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			/*
				For the forward iteration test, we'll actually check that
				the iterator covers the expect range of records.  For the
				reverse iteration test, we'll take a simplified approach.
			*/

			// Iterate through records once, then return nil
			iter := mock_routing.NewMockIterator(ctrl)
			for _, r := range recs {
				iter.EXPECT().Next().Return(r).Times(1)
			}
			iter.EXPECT().Next().Return(nil).Times(1)

			query := mock_routing.NewMockQuery(ctrl)
			query.EXPECT().
				Get(gomock.Any()).
				Return(iter, nil).
				Times(1)

			// Double-reverse so that we test the code path that
			// takes us *back* to a normal, forward-iterating view.
			v := view.View{Query: query}.Reverse().Reverse()

			it, err := v.Iter(view.All())
			require.NoError(t, err, "lookup should succeed")
			require.NotNil(t, it, "should return iterator")

			var ctr int
			for r := it.Next(); r != nil; r = it.Next() {
				want := routing.Record(recs[ctr])
				require.Equal(t, want, r, "should match record %d", ctr)
				ctr++
			}
		})

		t.Run("Reverse", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			/*
				Just test that reversing the view calls the expected query method.
				In addition to simplifying test code, it explores the code path in
				which a nil iterator is produced by the Selector.
			*/

			query := mock_routing.NewMockQuery(ctrl)
			query.EXPECT().
				GetReverse(gomock.Any()).
				Return(nil, nil).
				Times(1)

			v := view.View{Query: query}.Reverse()

			_, err := v.Iter(view.All())
			require.NoError(t, err, "lookup should succeed")
		})
	})
}

func TestSelector(t *testing.T) {
	t.Parallel()
	t.Helper()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	query := mock_routing.NewMockQuery(ctrl)

	/*
		Test that each selector calls the expected Query method.
	*/
	for _, tt := range []struct {
		name string
		sel  view.Selector
		call *gomock.Call
	}{
		{
			name: "All",
			sel:  view.All(),
			call: query.EXPECT().
				Get(gomock.Any()). // TODO: verify that record is expected
				Return(nil, nil).
				Times(1),
		},
		{
			name: "Match",
			sel:  view.Match(nil),
			call: query.EXPECT().
				Get(gomock.Any()).
				Return(nil, nil).
				Times(1),
		},
		{
			name: "From",
			sel:  view.From(nil),
			call: query.EXPECT().
				LowerBound(gomock.Any()).
				Return(nil, nil).
				Times(1),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.sel(query)
			assert.NoError(t, err)
		})
	}
}

func TestRangeQuery(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	/*
		A range query is a constraint placed upon the `From` selector.
		It is a common (and indeed, important) pattern, and so is given
		a test of its own.
	*/

	// We define a set of records with lexicographically ordered IDs.
	// This will allow us to define a range over the id index.
	recs := []*record{
		{id: peer.ID("foo")},
		{id: peer.ID("foobar")},
		{id: peer.ID("foobarbaz")},    // <- range must support dupes!
		{id: peer.ID("foobarbaz")},    // <- last item in the range
		{id: peer.ID("foobarbazqux")}, // <- last item to be iterated upon
		{id: peer.ID("foobarbazqux")}, // <- skipped
	}

	// Create a mock iterator that is expected to iterate through all
	// but the last record.  The second-to-last record will be returned
	// by the last call to Next(), which should cause the range iterator
	// to detect that the range has been exceeded, and return nil.  Once
	// the range iterator has detected that it is out-of-bounds, it will
	// cease to call the mock iterator's Next() method. Or so we hope...!
	iter := mock_routing.NewMockIterator(ctrl)
	iter.EXPECT().Next().Return(recs[0]).Times(1)
	iter.EXPECT().Next().Return(recs[1]).Times(1)
	iter.EXPECT().Next().Return(recs[2]).Times(1)
	iter.EXPECT().Next().Return(recs[3]).Times(1)
	iter.EXPECT().Next().Return(recs[4]).Times(1) // <- not returned!
	// NOTE:  the mock iterator is NOT expected to have a call to Next
	//        that returns nil! This is because the predicateIter will
	//        short-circuit it.

	// Next, we construct a query that returns the above iterator.
	query := mock_routing.NewMockQuery(ctrl)
	query.EXPECT().
		LowerBound(gomock.Any()).
		Return(iter, nil).
		Times(1)

	// We now define an index over id_prefix matching 'foo'.  On its own,
	// this would match *all* records in the iterator.
	min := mock_routing.NewMockIndex(ctrl)
	min.EXPECT().
		Match(gomock.Any()).
		DoAndReturn(func(r routing.Record) bool {
			return strings.HasPrefix(string(r.Peer()), "foo")
		}).
		AnyTimes()

	// Now we define an index that designates the upper bound on the range.
	// This matches the highest-order id that is part of the range.
	max := mock_routing.NewMockIndex(ctrl)
	max.EXPECT().
		Match(gomock.Any()).
		DoAndReturn(func(r routing.Record) bool {
			return string(r.Peer()) == "foobarbaz"
		}).
		AnyTimes()

	selector := view.Range(min, max)
	it, err := selector(query)
	require.NoError(t, err, "selector should not return error")
	require.NotNil(t, it, "selector should return iterator")

	for _, want := range recs[:4] {
		r := it.Next()
		require.NotNil(t, r, "iterator should not be exhausted")
		require.Equal(t, want, r, "should be record %s", string(want.id))
	}

	require.Nil(t, it.Next(), "iterator should be exhausted")
}

func TestFirst(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	recs := []*record{
		{id: peer.ID("foo")},
		{id: peer.ID("foobar")},
	}

	iter := mock_routing.NewMockIterator(ctrl)
	iter.EXPECT().Next().Return(recs[0]).Times(1)
	iter.EXPECT().Next().Return(recs[1]).Times(1) // <- skipped

	query := mock_routing.NewMockQuery(ctrl)
	query.EXPECT().
		Get(gomock.Any()). // <- what we're actually testing
		Return(iter, nil).
		Times(1)

	selector := view.All().Bind(view.First())
	it, err := selector(query)
	require.NoError(t, err)
	require.NotNil(t, it)

	require.Equal(t, routing.Record(recs[0]), it.Next(),
		"should return first record in iterator")
	require.Nil(t, it.Next(),
		"iterator should be expired")

}

func TestLimit(t *testing.T) {
	t.Parallel()

	/*
		Normal operation of Limit is already tested by TestFirst.
		Here, we just test the code path corresponding to invalid
		parameters.
	*/

	it, err := view.Limit(0)(nil)(nil)
	require.Error(t, err, "should reject invalid limit '0'")
	require.Nil(t, it, "should not return an iterator")

	it, err = view.Limit(-1)(nil)(nil)
	require.Error(t, err, "should reject invalid limit '0'")
	require.Nil(t, it, "should not return an iterator")
}

func TestWhere(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	recs := []*record{
		{id: peer.ID("foo")},
		{id: peer.ID("foobar")},
		{id: peer.ID("foobarbaz")},
		{id: peer.ID("quxbazbar")},
	}

	iter := mock_routing.NewMockIterator(ctrl)
	iter.EXPECT().Next().Return(recs[0]).Times(1) // <- skipped
	iter.EXPECT().Next().Return(recs[1]).Times(1)
	iter.EXPECT().Next().Return(recs[2]).Times(1) // <-skipped
	iter.EXPECT().Next().Return(recs[3]).Times(1)
	iter.EXPECT().Next().Return(nil).Times(1) // <- exhausted

	query := mock_routing.NewMockQuery(ctrl)
	query.EXPECT().
		Get(gomock.Any()). // <- what we're actually testing
		Return(iter, nil).
		Times(1)

	containsBar := matchFunc(func(r routing.Record) bool {
		return strings.Contains(string(r.Peer()), "bar")
	})

	selector := view.All().Bind(view.Where(containsBar))

	it, err := selector(query)
	require.NoError(t, err)
	require.NotNil(t, it)

	var got []routing.Record
	for r := it.Next(); r != nil; r = it.Next() {
		got = append(got, r)
	}

	require.Len(t, got, 3)
	require.NotContains(t, got, recs[0])
}

type matchFunc func(routing.Record) bool

func (match matchFunc) Match(r routing.Record) bool {
	return match(r)
}

type record struct {
	once sync.Once
	id   peer.ID
	seq  uint64
	ins  uint32
	host string
	meta routing.Meta
	ttl  time.Duration
}

func (r *record) init() {
	r.once.Do(func() {
		if r.id == "" {
			r.id = newPeerID()
		}

		if r.host == "" {
			r.host = newPeerID().String()[:16]
		}

		if r.ins == 0 {
			r.ins = rand.Uint32()
		}
	})
}

func (r *record) Peer() peer.ID {
	r.init()
	return r.id
}

func (r *record) Seq() uint64 { return r.seq }

func (r *record) Host() (string, error) {
	r.init()
	return r.host, nil
}

func (r *record) Instance() uint32 {
	r.init()
	return r.ins
}

func (r *record) TTL() time.Duration {
	if r.init(); r.ttl == 0 {
		return time.Second
	}

	return r.ttl
}

func (r *record) Meta() (routing.Meta, error) { return r.meta, nil }

func newPeerID() peer.ID {
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	sk, _, err := crypto.GenerateEd25519Key(rnd)
	if err != nil {
		panic(err)
	}

	id, err := peer.IDFromPrivateKey(sk)
	if err != nil {
		panic(err)
	}

	return id
}
