package pex_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/host"
	ps "github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wetware/casm/pkg/pex"
	mx "github.com/wetware/matrix/pkg"
)

const ns = "casm.pex.test"

func TestHostRegression(t *testing.T) {
	t.Parallel()

	/*
	 * This is a regression test to ensure the libp2p Host
	 * supports all necessary features.
	 */

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := mx.New(ctx).MustHost(ctx)
	defer h.Close()

	/*
	 * First we validate that the host provides a stateful
	 * event subscription for addres updates.  If not, the
	 * pex constructor will block indefinitely.
	 *
	 * See:  https://github.com/libp2p/go-libp2p/pull/1147
	 */

	s, err := h.EventBus().Subscribe(new(event.EvtLocalAddressesUpdated))
	require.NoError(t, err)

	select {
	case v := <-s.Out():
		require.NotNil(t, v)
		require.IsType(t, event.EvtLocalAddressesUpdated{}, v)
		ev := v.(event.EvtLocalAddressesUpdated)

		require.NotZero(t, ev.Current)
		require.NotZero(t, ev.SignedPeerRecord)
	case <-time.After(time.Millisecond * 100):
		t.Error("did not receive initial addrs")
	}

	/*
	 * Once the event has been received, the host's signed
	 * record should be contained within the address book,
	 * provided it satisfies peerstore.CertifiedAddrBook.
	 *
	 * If this fails, we may experience panics as type
	 * assertions fail.
	 */

	cb, ok := ps.GetCertifiedAddrBook(h.Peerstore())
	require.True(t, ok)

	env := cb.GetPeerRecord(h.ID())
	require.NotNil(t, env)
}

func TestPeerExchange_Init(t *testing.T) {
	t.Parallel()
	t.Helper()

	t.Run("Succeed", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		h := mx.New(ctx).MustHost(ctx)

		px, err := pex.New(ctx, h, pex.WithNamespace(ns))
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

		h := mx.New(ctx).MustHost(ctx, libp2p.NoListenAddrs)

		_, err := pex.New(ctx, h, pex.WithNamespace(ns))
		require.EqualError(t, err, "host not accepting connections")
	})
}

func TestPeerExchange_Join(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sim := mx.New(ctx)
	hs := sim.MustHostSet(ctx, 2)

	ps := make([]*pex.PeerExchange, len(hs))
	ss := make([]event.Subscription, len(hs))
	mx.Go(func(ctx context.Context, i int, h host.Host) (err error) {
		ps[i], err = pex.New(ctx, h, pex.WithNamespace(ns))
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
			require.IsType(t, pex.EvtViewUpdated{}, v)
			view := pex.View(v.(pex.EvtViewUpdated))

			// do we have an updated view?
			require.Len(t, view, len(hs)-1, // host doesn't include itself in view
				"unexpected length %d for host %d", len(view), i)

			// is the event the same as the local view?
			for ii, g := range ps[i].View() {
				require.True(t, view[ii].Envelope.Equal(g.Envelope))
			}

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

func TestPeerExchange_simulation(t *testing.T) {
	t.Parallel()
	t.Helper()

	const (
		n        = 8
		viewSize = n
		tick     = time.Millisecond * 10
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		sim = mx.New(ctx)
		hs  = sim.MustHostSet(ctx, n)
		xs  = make([]*pex.PeerExchange, n)
	)

	err := mx.
		/*
		 * Initialize a peer exchange for each host in hs
		 */
		Go(func(ctx context.Context, i int, h host.Host) (err error) {
			xs[i], err = pex.New(ctx, h,
				pex.WithMaxViewSize(viewSize),
				pex.WithNamespace(ns),
				pex.WithTick(tick))
			return
		}).
		/*
		 * Connect all hosts to h[0]
		 */
		Go(func(ctx context.Context, i int, _ host.Host) (err error) {
			if i > 0 {
				err = xs[i].Join(ctx, *host.InfoFromHost(hs[0]))
			}
			return
		}).
		/*
		 * Ensure views are eventually full.
		 */
		Go(func(ctx context.Context, i int, h host.Host) error {
			sub, err := h.EventBus().Subscribe(new(pex.EvtViewUpdated))
			if err != nil {
				return err
			}
			defer sub.Close()

			for {
				select {
				case v := <-sub.Out():
					view := pex.View(v.(pex.EvtViewUpdated))

					if view.Len() == n {
						return nil
					}

				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}).Err(ctx, hs)

	require.NoError(t, err)
}
