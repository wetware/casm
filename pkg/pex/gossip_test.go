package pex_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/record"
	"github.com/wetware/casm/pkg/pex"

	"github.com/stretchr/testify/require"
	mx "github.com/wetware/matrix/pkg"
)

func TestGossipRecord_MarshalUnmarshal(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := mx.New(ctx).MustHost(ctx)

	sub, err := h.EventBus().Subscribe(new(event.EvtLocalAddressesUpdated))
	require.NoError(t, err)

	var want pex.GossipRecord
	select {
	case v, ok := <-sub.Out():
		require.True(t, ok, "event bus closed unexpectedly")
		require.NotNil(t, v, "event was nil")

		want, err = pex.NewGossipRecordFromEvent(v.(event.EvtLocalAddressesUpdated))
		require.NoError(t, err)

	case <-time.After(time.Second):
		t.Error("timeout waiting for signed record")
		t.FailNow()
	}

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

	sub, err := h.EventBus().Subscribe(new(event.EvtLocalAddressesUpdated))
	require.NoError(t, err)

	var want pex.GossipRecord
	select {
	case v, ok := <-sub.Out():
		require.True(t, ok, "event bus closed unexpectedly")
		require.NotNil(t, v, "event was nil")

		want, err = pex.NewGossipRecordFromEvent(v.(event.EvtLocalAddressesUpdated))
		require.NoError(t, err)

	case <-time.After(time.Second):
		t.Error("timeout waiting for signed record")
		t.FailNow()
	}

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
	mx.Go(func(ctx context.Context, i int, h host.Host) error {
		sub, err := h.EventBus().Subscribe(new(event.EvtLocalAddressesUpdated))
		if err != nil {
			return err
		}

		select {
		case v := <-sub.Out():
			view[i], err = pex.NewGossipRecordFromEvent(v.(event.EvtLocalAddressesUpdated))
			return err

		case <-time.After(time.Second):
			return fmt.Errorf("timeout waiting for host %d", i)
		}
	}).Must(ctx, mx.Selection{h0, h1})

	b, err := view.MarshalRecord()
	require.NoError(t, err)
	require.NotEmpty(t, b)

	var remote pex.View
	err = remote.UnmarshalRecord(b)
	require.NoError(t, err)
}

func TestView_SealUnseal(t *testing.T) {
	t.Parallel()
	t.Helper()

	t.Run("Succeed", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sim := mx.New(ctx)

		hs := sim.MustHostSet(ctx, 2)
		sender := hs[len(hs)-1] // last record
		view := mustTestView(hs)

		// records from peers other than the sender MUST have hop > 0 in
		// order to pass validation.
		incrHops(view[:len(view)-1])

		// marshal
		want, err := record.Seal(&view, privkey(sender))
		require.NoError(t, err)

		b, err := want.Marshal()
		require.NoError(t, err)
		require.NotNil(t, b)

		// unmarshal
		var remote pex.View
		got, err := record.ConsumeTypedEnvelope(b, &remote)
		require.NoError(t, err)
		require.True(t, got.Equal(want))

		err = remote.Validate(got)
		require.NoError(t, err)
	})

	t.Run("Validation", func(t *testing.T) {
		t.Helper()
		t.Parallel()

		t.Run("sender_invalid_hop_range", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			sim := mx.New(ctx)

			hs := sim.MustHostSet(ctx, 2)
			sender := hs[len(hs)-1] // last record
			view := mustTestView(hs)

			/*
			 * N.B.:  we increment hops for _all_ records, including the
			 *        sender.  This should be caught in validation.
			 */
			incrHops(view)

			// marshal
			want, err := record.Seal(&view, privkey(sender))
			require.NoError(t, err)

			b, err := want.Marshal()
			require.NoError(t, err)
			require.NotNil(t, b)

			// unmarshal
			var remote pex.View
			got, err := record.ConsumeTypedEnvelope(b, &remote)
			require.NoError(t, err)
			require.True(t, got.Equal(want))

			err = remote.Validate(got)
			require.ErrorAs(t, err, &pex.ValidationError{})
			require.ErrorIs(t, err, pex.ErrInvalidRange)
		})

		t.Run("invalid_hop_range", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			sim := mx.New(ctx)

			hs := sim.MustHostSet(ctx, 2)
			sender := hs[len(hs)-1] // last record
			view := mustTestView(hs)

			/*
			 * N.B.:  we don't increment the hops here; this should be
			 *        caught in validation.
			 */

			// marshal
			want, err := record.Seal(&view, privkey(sender))
			require.NoError(t, err)

			b, err := want.Marshal()
			require.NoError(t, err)
			require.NotNil(t, b)

			// unmarshal
			var remote pex.View
			got, err := record.ConsumeTypedEnvelope(b, &remote)
			require.NoError(t, err)
			require.True(t, got.Equal(want))

			err = remote.Validate(got)
			require.ErrorAs(t, err, &pex.ValidationError{})
			require.ErrorIs(t, err, pex.ErrInvalidRange)
		})

		t.Run("last_record_not_signed_by_sender", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			sim := mx.New(ctx)

			hs := sim.MustHostSet(ctx, 2)
			notSender := hs[0]
			view := mustTestView(hs)

			// marshal - note that we sign with the WRONG host key.
			want, err := record.Seal(&view, privkey(notSender))
			require.NoError(t, err)

			b, err := want.Marshal()
			require.NoError(t, err)
			require.NotNil(t, b)

			// unmarshal
			var remote pex.View
			got, err := record.ConsumeTypedEnvelope(b, &remote)
			require.NoError(t, err)
			require.True(t, got.Equal(want))

			err = remote.Validate(got)
			require.ErrorAs(t, err, &pex.ValidationError{})
			require.ErrorIs(t, err, record.ErrInvalidSignature)
		})
	})
}

func mustTestView(hs []host.Host) pex.View {
	view := make(pex.View, len(hs))
	mx.Go(func(ctx context.Context, i int, h host.Host) error {
		sub, err := h.EventBus().Subscribe(new(event.EvtLocalAddressesUpdated))
		if err != nil {
			return err
		}

		select {
		case v := <-sub.Out():
			view[i], err = pex.NewGossipRecordFromEvent(v.(event.EvtLocalAddressesUpdated))
			return err

		case <-time.After(time.Second):
			return fmt.Errorf("timeout waiting for host %d", i)
		}
	}).Must(context.Background(), hs)
	return view
}

func privkey(h host.Host) crypto.PrivKey {
	return h.Peerstore().PrivKey(h.ID())
}

func incrHops(v pex.View) {
	for i := range v {
		v[i].Hop++ // need to use indexing; slice of non-ptr.
	}
}
