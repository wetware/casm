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

const ns = "casm.pex.test"

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

		px, err := pex.New(mx.New(ctx).MustHost(ctx), pex.WithNamespace(ns))
		require.NoError(t, err)

		assert.Equal(t, ns, px.String(), "unexpected namespace")
		assert.Empty(t, px.View(), "initialized view is non-empty")

		err = px.Close()
		assert.NoError(t, err, "error closing PeerExchange")
	})

	t.Run("Fail_no_addrs", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sim := mx.New(ctx)

		_, err := pex.New(sim.MustHost(ctx, libp2p.NoListenAddrs), pex.WithNamespace(ns))
		require.EqualError(t, err, "host not accepting connections")
	})
}

func TestPeerExchange_Join(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sim := mx.New(ctx)
	hs := sim.MustHostSet(ctx, 2)

	ps := make([]pex.PeerExchange, len(hs))
	ss := make([]event.Subscription, len(hs))
	mx.Go(func(ctx context.Context, i int, h host.Host) (err error) {
		ps[i], err = pex.New(h, pex.WithNamespace(ns))
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
		clusterSize = 64
		tick        = time.Microsecond * 1
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		sim = mx.New(ctx)
		hs  = sim.MustHostSet(ctx, clusterSize)
		xs  = make([]pex.PeerExchange, clusterSize)
	)

	mx. // initialize a peer exchange for each host in hs
		Go(func(ctx context.Context, i int, h host.Host) (err error) {
			xs[i], err = pex.New(h, pex.WithNamespace(ns), pex.WithTick(tick))
			return
		}).
		// join all hosts in a ring topology
		Go(func(ctx context.Context, i int, _ host.Host) (err error) {
			h := hs[len(hs)-1]
			if i > 0 {
				h = hs[i-1]
			}

			return xs[i].Join(ctx, *host.InfoFromHost(h))
		}).
		Must(ctx, hs)

	t.Run("ViewsAreEventuallyFull", func(t *testing.T) {
		err := mx.Go(func(ctx context.Context, i int, h host.Host) error {
			sub, err := h.EventBus().Subscribe(new(pex.EvtViewUpdated))
			if err != nil {
				return err
			}
			defer sub.Close()

			for {
				select {
				case v := <-sub.Out():
					view := pex.View(v.(pex.EvtViewUpdated))

					if view.Len() == 32 {
						return nil
					}

				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}).Err(ctx, hs)

		require.NoError(t, err)
	})

	t.Run("DeadPeersEventuallyPurged", func(t *testing.T) {
		t.Skip("NOT IMPLEMENTED") // TODO
	})

}
