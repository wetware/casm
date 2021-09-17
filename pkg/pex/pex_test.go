package pex_test

import (
	"context"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	ps "github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wetware/casm/pkg/boot"
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

		px, err := pex.New(ctx, h)
		require.NoError(t, err)

		peers, err := px.FindPeers(ctx, ns)
		require.NoError(t, err)

		_, ok := <-peers
		require.False(t, ok)

		err = px.Close()
		assert.NoError(t, err, "error closing PeerExchange")
	})

	t.Run("Fail_no_addrs", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		h := mx.New(ctx).MustHost(ctx, libp2p.NoListenAddrs)

		_, err := pex.New(ctx, h)
		require.EqualError(t, err, "host not accepting connections")
	})
}

func TestPeerExchange_join(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sim := mx.New(ctx)
	hs := sim.MustHostSet(ctx, 2)
	ps := make([]*pex.PeerExchange, len(hs))
	is := make([]peer.AddrInfo, len(hs))

	mx.
		Go(func(ctx context.Context, i int, h host.Host) (err error) {
			is[i] = *host.InfoFromHost(h)
			ps[i], err = pex.New(ctx, h)
			return
		}).
		Go(func(ctx context.Context, i int, h host.Host) error {
			if i == 0 {
				is[1] = *host.InfoFromHost(h)
			} else {
				is[0] = *host.InfoFromHost(h)
			}
			return nil
		}).
		Go(func(ctx context.Context, i int, h host.Host) error {
			joinCtx, joinCtxCancel := context.WithTimeout(ctx, time.Second)
			defer joinCtxCancel()

			if i == 0 {
				return ps[i].Join(joinCtx, ns, is[i])
			}

			return nil
		}).
		Go(func(ctx context.Context, i int, h host.Host) error {
			// TODO:  proper synchronization to ensure the 'Join()' call has completed
			time.Sleep(time.Millisecond)

			ch, err := ps[i].FindPeers(ctx, ns)
			require.NoError(t, err)

			info, ok := <-ch
			require.True(t, ok)
			require.Equal(t, is[i].ID, info.ID)
			return nil
		}).Must(ctx, hs)
}

func TestAdvertise(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hs := mx.New(ctx).MustHostSet(ctx, 8)
	px := make([]*pex.PeerExchange, len(hs))
	as := make(boot.StaticAddrs, len(hs))

	mx.
		Go(func(ctx context.Context, i int, h host.Host) (err error) {
			as[i] = *host.InfoFromHost(h)
			return
		}).
		Go(func(ctx context.Context, i int, h host.Host) (err error) {
			b := make(boot.StaticAddrs, 0, len(as)-1)
			for _, info := range as {
				if info.ID != h.ID() {
					b = append(b, info)
				}
			}

			px[i], err = pex.New(ctx, h, pex.WithDiscovery(b))
			return
		}).Must(ctx, hs)

	ttl, err := px[0].Advertise(ctx, ns)
	require.NoError(t, err)
	require.NotEqual(t, ps.PermanentAddrTTL, ttl,
		"pex SHOULD NOT use the underlying discovery service's TTL")
}

// func TestPeerExchange_simulation(t *testing.T) {
// 	t.Parallel()

// 	ctx, cancel := context.WithCancel(context.Background())
// 	defer cancel()

// 	const (
// 		n = 8 // number of peers in cluster
// 		k = 7 // max number of records in view

// 		tick = time.Millisecond
// 		ttl  = tick * 10
// 	)

// 	var (
// 		opt = []discovery.Option{
// 			discovery.TTL(ttl),
// 			discovery.Limit(k),
// 		}

// 		sim = mx.New(ctx)
// 		hs  = sim.MustHostSet(ctx, 2)
// 		ps  = make([]*pex.PeerExchange, len(hs))
// 		b   = make(boot.StaticAddrs, len(hs))
// 	)

// 	err := mx.
// 		Go(func(ctx context.Context, i int, h host.Host) (err error) {
// 			b[i] = *host.InfoFromHost(h)
// 			return
// 		}).
// 		Go(func(ctx context.Context, i int, h host.Host) (err error) {
// 			ps[i], err = pex.New(ctx, h, pex.WithDiscovery(b, opt...))
// 			return
// 		}).
// 		// start advertiser loop
// 		Go(func(ctx context.Context, i int, h host.Host) error {
// 			next, err := ps[i].Advertise(ctx, ns, opt...)
// 			if err == nil {
// 				sim.Clock().Ticker()
// 				go func() {
// 					for {
// 						select {
// 						case <-time.After(next):
// 							next, err = ps[i].Advertise(ctx, ns, opt...)
// 							require.NoError(t, err)
// 						case <-ctx.Done():
// 							return
// 						}
// 					}
// 				}()
// 			}

// 			return err
// 		}).
// 		Go(func(ctx context.Context, i int, h host.Host) error {
// 			sim.Clock().Ticker()
// 		}).Err(ctx, hs)

// 	require.NoError(t, err)
// }
