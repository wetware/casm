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

const vsize = 4 // max view size

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
		// {
		// 	// ...
		// },
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
			Opt: []Option{
				WithNamespace("test"),
				WithMaxViewSize(vsize)},
		}),
		fx.Provide(
			newConfig,
			newGossipStore,
			newHostComponents(mx.New(ctx).MustHost(ctx))),
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
	Local  view `name:"local"`
	Remote view `name:"remote"`
}

type params struct {
	fx.In

	Host   host.Host
	Store  *gossipStore
	Local  view `name:"local"`
	Remote view `name:"remote"`
}

func (p params) LocalRecord() *record.Envelope {
	return mustTestView([]host.Host{p.Host})[0].Envelope
}

func shouldHaveViewSize_vsize(t *testing.T, p params) {
	err := p.Store.SetLocalRecord(p.LocalRecord())
	require.NoError(t, err)

	// When the current view is full (= n) ...
	err = p.Store.MergeAndStore(p.Local)
	require.NoError(t, err)

	// ... and we merge a remote view ...
	err = p.Store.MergeAndStore(p.Remote)
	require.NoError(t, err)

	// ... the size of the resulting view should be n.
	gs, err := p.Store.Load()
	require.NoError(t, err)
	require.Len(t, gs, vsize)
}

func mkValidView(n int) view {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	view := mustTestView(mx.New(ctx).MustHostSet(ctx, n))
	view[:n-1].incrHops() // last record must have hops=0

	return view
}
