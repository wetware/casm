package pex_test

import (
	"context"
	"math/rand"
	"testing"

	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/record"
	"github.com/wetware/casm/pkg/pex"

	"github.com/stretchr/testify/require"
	mx "github.com/wetware/matrix/pkg"
)

var r = rand.New(rand.NewSource(42))

func TestNewGossipRecord(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := mx.New(ctx).MustHost(ctx)
	waitReady(h)

	rec, err := pex.NewGossipRecord(h)
	require.NoError(t, err)

	require.True(t, rec.Validate())

	rec.PeerID = newPeerID()
	require.False(t, rec.Validate())
}

func TestGossipRecord_MarshalUnmarshal(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := mx.New(ctx).MustHost(ctx)
	waitReady(h)

	want, err := pex.NewGossipRecord(h)
	require.NoError(t, err)

	// marshal
	b, err := want.MarshalRecord()
	require.NoError(t, err)
	require.NotEmpty(t, b)

	// unmarshal
	got := new(pex.GossipRecord)
	err = got.UnmarshalRecord(b)
	require.NoError(t, err)

	// validate
	require.Equal(t, want.Hop, got.Hop)
	require.Equal(t, want.PeerID, got.PeerID)

	require.NotNil(t, got.Addrs)
	require.Len(t, got.Addrs, len(want.Addrs))

	for i, expect := range want.Addrs {
		require.True(t, expect.Equal(got.Addrs[i]))
	}
}

func TestGossipRecord_SealUnseal(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := mx.New(ctx).MustHost(ctx)
	waitReady(h)

	want, err := pex.NewGossipRecord(h)
	require.NoError(t, err)

	// seal
	e, err := record.Seal(&want, h.Peerstore().PrivKey(h.ID()))
	require.NoError(t, err)
	require.NotNil(t, e)

	// marshal
	b, err := e.Marshal()
	require.NoError(t, err)
	require.NotEmpty(t, b)

	// unseal
	var got pex.GossipRecord
	e2, err := record.ConsumeTypedEnvelope(b, &got)
	require.NoError(t, err)
	require.NotNil(t, e2)

	require.True(t, e2.Equal(e), "decoded envelope not equal to original")
}

func TestView_MarshalUnmarshal(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sim := mx.New(ctx)

	h0 := sim.MustHost(ctx)
	h1 := sim.MustHost(ctx)

	view := make(pex.View, 2)
	mx.
		Go(func(ctx context.Context, i int, h host.Host) error {
			waitReady(h)
			return nil
		}).
		Go(func(ctx context.Context, i int, h host.Host) (err error) {
			view[i], err = pex.NewGossipRecord(h)
			return
		}).Must(ctx, mx.Selection{h0, h1})

	b, err := view.Marshal()
	require.NoError(t, err)
	require.NotEmpty(t, b)

	var remote pex.View
	err = remote.Unmarshal(b)
	require.NoError(t, err)

	for i, got := range remote {
		require.True(t, view[i].Envelope.Equal(got.Envelope))
		require.Equal(t, view[i].Hop, got.Hop)
	}
}

func TestView_validation(t *testing.T) {
	t.Parallel()
	t.Helper()

	t.Run("Succeed", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sim := mx.New(ctx)

		hs := sim.MustHostSet(ctx, 2)
		view := mustTestView(hs)

		// records from peers other than the sender MUST have hop > 0 in
		// order to pass validation.
		incrHops(view[:len(view)-1])

		err := view.Validate()
		require.NoError(t, err)
	})

	t.Run("Fail", func(t *testing.T) {
		t.Helper()
		t.Parallel()

		t.Run("sender_invalid_hop_range", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			sim := mx.New(ctx)

			hs := sim.MustHostSet(ctx, 2)
			view := mustTestView(hs)

			/*
			 * N.B.:  we increment hops for _all_ records, including the
			 *        sender.  This should be caught in validation.
			 */
			incrHops(view)

			err := view.Validate()
			require.ErrorAs(t, err, &pex.ValidationError{})
			require.ErrorIs(t, err, pex.ErrInvalidRange)
		})

		t.Run("invalid_hop_range", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			sim := mx.New(ctx)

			hs := sim.MustHostSet(ctx, 2)

			/*
			 * N.B.:  we don't increment the hops here; this should be
			 *        caught in validation.
			 */

			err := mustTestView(hs).Validate()
			require.ErrorAs(t, err, &pex.ValidationError{})
			require.ErrorIs(t, err, pex.ErrInvalidRange)
		})

		t.Run("invalid_signature", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			sim := mx.New(ctx)

			hs := sim.MustHostSet(ctx, 1)
			view := mustTestView(hs)

			view[0].PeerID = newPeerID()
			err := view.Validate()
			require.ErrorAs(t, err, &pex.ValidationError{})
		})
	})
}

func mustTestView(hs []host.Host) pex.View {
	view := make(pex.View, len(hs))
	mx.
		Go(func(ctx context.Context, i int, h host.Host) error {
			waitReady(h)
			return nil
		}).
		Go(func(ctx context.Context, i int, h host.Host) (err error) {
			view[i], err = pex.NewGossipRecord(h)
			return
		}).Must(context.Background(), hs)
	return view
}

func incrHops(v pex.View) {
	for i := range v {
		v[i].Hop++ // need to use indexing; slice of non-ptr.
	}
}

func waitReady(h host.Host) {
	sub, err := h.EventBus().Subscribe(new(event.EvtLocalAddressesUpdated))
	if err != nil {
		panic(err)
	}
	defer sub.Close()

	<-sub.Out()
}

func newPeerID() peer.ID {
	// use non-cryptographic source; it's just a test.
	sk, _, err := crypto.GenerateECDSAKeyPair(r)
	if err != nil {
		panic(err)
	}

	id, err := peer.IDFromPrivateKey(sk)
	if err != nil {
		panic(err)
	}

	return id
}
