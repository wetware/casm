package pex

import (
	"context"
	"fmt"
	"math/rand"
	"testing"

	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/record"
	"github.com/stretchr/testify/require"
	mx "github.com/wetware/matrix/pkg"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
)

const (
	ns    = "casm.pex.test"
	vsize = 30 // max view size
)

func TestMerge(t *testing.T) {
	t.Parallel()
	t.Helper()

	for _, tt := range []struct {
		name string
		test func(*testing.T, params)
	}{
		{
			name: fmt.Sprintf("should_have_view_size=%d", vsize),
			test: shouldHaveViewSize_vsize,
		},
		{
			name: "should_keep_records_with_equal_hop_seq",
			test: shouldKeepRecordsWithEqualHopSeq,
		},
		{
			name: "should_retain_higher_seq",
			test: shouldRetainHigherSeq,
		},
		{
			name: "should_retain_lower_hop",
			test: shouldRetainLowerHop,
		},
		{
			name: "should_retain_higher_seq_despite_lower_hop",
			test: shouldRetainHigherSeqDespiteLowerHop,
		},
		{
			name: "should_swap",
			test: shouldSwap,
		},
		{
			name: "should_retain",
			test: shouldRetain,
		},
		{
			name: "should_not_retain",
			test: shouldNotRetain,
		},
		{
			name: "should_sort_to_push",
			test: shouldSortToPush,
		},
	} {
		runner(t, tt.name, tt.test)
	}
}

func runner(t *testing.T, name string, f func(t *testing.T, p params)) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	app := fxtest.New(t, fx.NopLogger,
		fx.Supply(out{
			Local:  mkValidView(vsize),
			Remote: mkValidView(vsize),
			Opt:    []Option{WithGossipParams(GossipParams{vsize, -1, -1, -1})},
		}),
		fx.Provide(
			newConfig,
			newDiscover,
			newPeerExchange,
			supply(ctx, mx.New(ctx).MustHost(ctx))),
		fx.Invoke(func(p params) {
			t.Run(name, func(t *testing.T) {
				t.Helper()
				f(t, p)
			})
		}))

	err := app.Start(ctx)
	require.NoError(t, err)

	err = app.Stop(ctx)
	require.NoError(t, err)
}

type out struct {
	fx.Out

	Opt    []Option
	Local  gossipSlice `name:"local"`
	Remote gossipSlice `name:"remote"`
}

type params struct {
	fx.In

	Host   host.Host
	Local  gossipSlice `name:"local"`
	Remote gossipSlice `name:"remote"`
	PeX    *PeerExchange
}

func (p params) LocalRecord() *record.Envelope {
	return mustGossipSlice([]host.Host{p.Host})[0].Envelope
}

func shouldSortToPush(t *testing.T, p params) {
	err := p.PeX.setLocalRecord(p.LocalRecord())
	require.NoError(t, err)

	n := p.PeX.namespace(ns)

	// Set random HOPs
	for _, rec := range p.Local {
		rec.g.SetHop(uint64(rand.Intn(100-1) + 1))
	}
	n.Store(gossipSlice{}, p.Local)

	// Retrieve sorted records
	recs, err := n.RecordsSortedToPush()
	require.NoError(t, err)

	// Check records are sorted
	youngest, oldest := recs.Bind(head(len(recs)-n.gossip.R)), recs.Bind(tail(n.gossip.R))
	oldestYoungest := youngest.Bind(sorted())[len(youngest)-1]
	for _, old := range oldest {
		require.True(t, oldestYoungest.Hop() <= old.Hop())
	}
}

func shouldHaveViewSize_vsize(t *testing.T, p params) {
	err := p.PeX.setLocalRecord(p.LocalRecord())
	require.NoError(t, err)

	n := p.PeX.namespace(ns)

	// When the current view is full (= n) ...
	err = n.MergeAndStore(p.Local, p.Local)
	require.NoError(t, err)

	// ... and we merge a remote view ...
	err = n.MergeAndStore(p.Local, p.Remote)
	require.NoError(t, err)

	// ... the size of the resulting view should be n.
	gs, err := n.View()
	require.NoError(t, err)
	require.Len(t, gs, vsize)
}

func shouldKeepRecordsWithEqualHopSeq(t *testing.T, p params) {
	err := p.PeX.setLocalRecord(p.LocalRecord())
	require.NoError(t, err)

	n := p.PeX.namespace(ns)

	// Copy Local records to Remote
	for i, lrec := range p.Local {
		p.Remote[i].Seq = lrec.Seq
		p.Remote[i].g.SetHop(lrec.Hop()) // Redundant because initially Hop=0
		p.Remote[i].PeerID = lrec.PeerID
	}
	p.Remote[len(p.Remote)-1].g.SetHop(0)

	err = n.MergeAndStore(p.Local, p.Local)
	require.NoError(t, err)
	err = n.MergeAndStore(p.Local, p.Remote)
	require.NoError(t, err)

	// ... the size of the resulting view should be n.
	gs, err := n.View()
	require.NoError(t, err)
	require.Len(t, gs, vsize)
}

func shouldSwap(t *testing.T, p params) {
	err := p.PeX.setLocalRecord(p.LocalRecord())
	require.NoError(t, err)

	n := p.PeX.namespace(ns)

	local := p.Local.Bind(sorted())

	n.gossip.S = 2
	err = n.MergeAndStore(local, p.Remote)
	require.NoError(t, err)
	gs, err := n.Records()
	require.NoError(t, err)

	merge := local.
		Bind(merged(p.Remote)).
		Bind(isNot(n.id))
	s := min(n.gossip.S, max(len(merge)-n.gossip.C, 0))

	for _, rec := range merge[:s] {
		_, found := gs.find(rec)
		require.False(t, found)
	}
}

func shouldRetain(t *testing.T, p params) {
	err := p.PeX.setLocalRecord(p.LocalRecord())
	require.NoError(t, err)

	n := p.PeX.namespace(ns)

	local := p.Local.Bind(sorted())

	merge := local.
		Bind(merged(p.Remote)).
		Bind(isNot(n.id))

	r := min(min(n.gossip.R, n.gossip.C), len(merge))
	oldest := merge.Bind(sorted()).Bind(tail(r))

	n.gossip.S = 0
	n.gossip.D = 0
	err = n.MergeAndStore(local, p.Remote)
	require.NoError(t, err)
	gs, err := n.RecordsSortedToPush()
	require.NoError(t, err)
	for _, rec := range oldest {
		_, found := gs.find(rec)
		require.True(t, found)
	}
}

func shouldNotRetain(t *testing.T, p params) {
	err := p.PeX.setLocalRecord(p.LocalRecord())
	require.NoError(t, err)

	n := p.PeX.namespace(ns)

	local := p.Local

	merge := local.
		Bind(merged(p.Remote)).
		Bind(isNot(n.id))

	r := min(min(n.gossip.R, n.gossip.C), len(merge))
	oldest := merge.Bind(sorted()).Bind(tail(r))

	n.gossip.S = 0
	n.gossip.D = 1
	err = n.MergeAndStore(local, p.Remote)
	require.NoError(t, err)
	gs, err := n.RecordsSortedToPush()
	require.NoError(t, err)
	for _, rec := range oldest {
		_, found := gs.find(rec)
		require.False(t, found)
	}
}

func shouldRetainHigherSeq(t *testing.T, p params) {
	t.Skip("Skipping ... (NOT IMPLEMENTED)")
}

func shouldRetainLowerHop(t *testing.T, p params) {
	t.Skip("Skipping ... (NOT IMPLEMENTED)")
}

func shouldRetainHigherSeqDespiteLowerHop(t *testing.T, p params) {
	t.Skip("Skipping ... (NOT IMPLEMENTED)")
}

func mkValidView(n int) gossipSlice {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	gs := mustGossipSlice(mx.New(ctx).MustHostSet(ctx, n))
	gs[:n-1].incrHops() // last record must have hops=0

	return gs
}
