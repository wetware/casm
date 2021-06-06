package pex_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wetware/casm/pkg/pex"
	mx "github.com/wetware/matrix/pkg"
)

func TestHost_LocalAddressesUpdated_stateful(t *testing.T) {
	t.Parallel()

	/*
	 * This is a regression test to ensure 'LocalAddressesUpdated' is
	 * stateful.  See:  https://github.com/libp2p/go-libp2p/pull/1147.
	 */

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sim := mx.New(ctx)

	h0 := sim.MustHost(ctx)
	defer h0.Close()

	s, err := h0.EventBus().Subscribe(new(event.EvtLocalAddressesUpdated))
	require.NoError(t, err)

	select {
	case v := <-s.Out():
		require.NotNil(t, v)
		require.NotZero(t, v.(event.EvtLocalAddressesUpdated).Current)
	case <-time.After(time.Millisecond * 100):
		t.Error("did not receive initial addrs")
	}
}

func TestPeerExchange_Init(t *testing.T) {
	t.Parallel()
	t.Helper()

	t.Run("Succeed", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		const ns = "test"
		px, err := pex.New(mx.New(ctx).MustHost(ctx), ns)
		require.NoError(t, err)

		assert.Equal(t, ns, px.String(), "unexpected namespace")
		assert.NotNil(t, px.Process(), "nil process")
		assert.Empty(t, px.View(), "initialized view is non-empty")

		err = px.Close()
		assert.NoError(t, err, "error closing PeerExchange")
	})

	t.Run("Fail_no_addrs", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sim := mx.New(ctx)

		_, err := pex.New(sim.MustHost(ctx, libp2p.NoListenAddrs), "test")
		require.EqualError(t, err, "host not accepting connections")
	})
}

func TestPeerExchange_Join(t *testing.T) {
	t.Parallel()

	const ns = "test"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sim := mx.New(ctx)
	hs := sim.MustHostSet(ctx, 2)

	ps := make([]*pex.PeerExchange, len(hs))
	ss := make([]event.Subscription, len(hs))
	mx.Go(func(ctx context.Context, i int, h host.Host) (err error) {
		ps[i], err = pex.New(h, ns)
		return
	}).Go(func(ctx context.Context, i int, h host.Host) (err error) {
		ss[i], err = h.EventBus().Subscribe(new(pex.EvtViewUpdated))
		return
	}).Must(ctx, hs)

	joinCtx, joinCtxCancel := context.WithTimeout(ctx, time.Second)
	defer joinCtxCancel()

	err := ps[0].Join(joinCtx, *host.InfoFromHost(hs[1]))
	require.NoError(t, err)

	err = mx.Go(func(ctx context.Context, i int, h host.Host) (err error) {
		// did we get the event?
		select {
		case v, ok := <-ss[i].Out():
			require.True(t, ok)
			require.NotEmpty(t, v.(pex.EvtViewUpdated))

			// do we have an updated view?
			view := ps[i].View()
			require.Len(t, view, len(hs)-1, // host doesn't include itself in view
				"unexpected length %d for host %d", len(view), i)

		case <-ctx.Done():
			err = ctx.Err()
		}

		if err != nil {
			err = fmt.Errorf("%d: %w", i, err)
		}

		return
	}).Err(ctx, hs)

	require.NoError(t, err)
}

func TestPeerExchange_Simulation(t *testing.T) {
	t.Parallel()
	t.Helper()

	const (
		clusterSize = 32
		ns          = "casm.pex.test"

		tick        = time.Millisecond * 1
		simDuration = time.Second * 30
		sampleRate  = time.Millisecond * 100
	)

	dl, ok := t.Deadline()
	if ok && simDuration < time.Until(dl) {
		t.Skipf("simulation skipped due to test timeout (max: -timeout=%v)",
			simDuration)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		sim = mx.New(ctx)
		hs  = sim.MustHostSet(ctx, clusterSize)
		xs  = make([]*pex.PeerExchange, clusterSize)
	)

	// initialize a peer exchange for each host in hs, and join
	// the namespace.
	mx.Go(func(ctx context.Context, i int, h host.Host) (err error) {
		xs[i], err = pex.New(h, ns, pex.WithTick(tick))
		return
	}).Go(func(ctx context.Context, i int, _ host.Host) (err error) {
		if i > 0 {
			err = xs[i].Join(ctx, *host.InfoFromHost(hs[i-1]))
		}
		return
	}).Must(ctx, hs)

	t.Run("ViewsAreEventuallyFull", func(t *testing.T) {
		assert.Eventually(t, func() bool {
			for i, px := range xs {
				t.Logf("peer %s:  %d", hs[i].ID().Pretty(), px.View().Len())
				if px.View().Len() != pex.ViewSize {
					return false
				}
			}
			return true
		}, simDuration, sampleRate)
	})

	// TODO:  test that dead peers eventually disappear from cluster
}
