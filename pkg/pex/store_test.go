package pex

import (
	"context"
	"fmt"
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
	vsize = 4 // max view size
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
			Opt:    []Option{WithMaxViewSize(vsize)},
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

func shouldHaveViewSize_vsize(t *testing.T, p params) {
	err := p.PeX.setLocalRecord(p.LocalRecord())
	require.NoError(t, err)

	n := p.PeX.namespace(ns)

	// When the current view is full (= n) ...
	err = n.MergeAndStore(p.Local)
	require.NoError(t, err)

	// ... and we merge a remote view ...
	err = n.MergeAndStore(p.Remote)
	require.NoError(t, err)

	// ... the size of the resulting view should be n.
	gs, err := n.View()
	require.NoError(t, err)
	require.Len(t, gs, vsize)
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
