package pex

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"capnproto.org/go/capnp/v3"
	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/libp2p/go-libp2p-core/record"

	"github.com/stretchr/testify/require"
	mx "github.com/wetware/matrix/pkg"
)

func TestNewGossipRecord(t *testing.T) {
	t.Parallel()
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := mx.New(ctx).MustHost(ctx)
	e, err := getSignedRecord(h)
	require.NoError(t, err)

	g, err := NewGossipRecord(e)
	require.NoError(t, err)
	require.NotNil(t, g.Envelope)

	require.Zero(t, g.Hop())
	g.IncrHop()
	require.Equal(t, uint64(1), g.Hop())

	r, err := g.Envelope.Record()
	require.NoError(t, err)
	require.NotNil(t, r)
	require.IsType(t, &peer.PeerRecord{}, r)

	rec := r.(*peer.PeerRecord)
	rec.PeerID.MatchesPublicKey(g.PublicKey)
}

func TestGossipRecord_MarshalUnmarshal(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := mx.New(ctx).MustHost(ctx)
	e, err := getSignedRecord(h)
	require.NoError(t, err)

	want, err := NewGossipRecord(e)
	require.NoError(t, err)
	require.NotZero(t, want.Seq)

	// increment hop to ensure that it is properly encoded.
	want.IncrHop()
	require.Equal(t, uint64(1), want.Hop())

	var buf bytes.Buffer

	// marshal
	err = capnp.NewPackedEncoder(&buf).Encode(want.Message())
	require.NoError(t, err)

	t.Logf("message size (packed): %d", buf.Len())

	// unmarshal
	msg, err := capnp.NewPackedDecoder(&buf).Decode()
	require.NoError(t, err)

	var got GossipRecord
	err = got.ReadMessage(msg)
	require.NoError(t, err)

	// validate
	require.Equal(t, want.Hop(), got.Hop())
	require.Equal(t, want.PeerID, got.PeerID)
	require.True(t, want.Envelope.Equal(got.Envelope))

	require.NotNil(t, got.Addrs)
	require.Len(t, got.Addrs, len(want.Addrs))
	for i, expect := range want.Addrs {
		require.True(t, expect.Equal(got.Addrs[i]))
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

	t.Run("SenderOnly", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sim := mx.New(ctx)

		hs := sim.MustHostSet(ctx, 1)
		view := mustTestView(hs)

		/*
		 * There is only one record (from the sender), so don't
		 * increment hops.  This should pass.
		 */

		err := view.Validate()
		require.NoError(t, err)
	})

	t.Run("Fail", func(t *testing.T) {
		t.Helper()
		t.Parallel()

		t.Run("empty", func(t *testing.T) {
			t.Parallel()

			var v view
			err := v.Validate()
			require.ErrorAs(t, err, &ValidationError{})
			require.EqualError(t, err, "empty view")
		})

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
			require.ErrorAs(t, err, &ValidationError{})
			require.ErrorIs(t, err, ErrInvalidRange)
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
			require.ErrorAs(t, err, &ValidationError{})
			require.ErrorIs(t, err, ErrInvalidRange)
		})
	})
}

func mustTestView(hs []host.Host) view {
	view := make([]*GossipRecord, len(hs))
	mx.
		Go(func(ctx context.Context, i int, h host.Host) error {
			waitReady(h)
			return nil
		}).
		Go(func(ctx context.Context, i int, h host.Host) error {
			e, err := getSignedRecord(h)
			if err == nil {
				view[i], err = NewGossipRecord(e)
			}

			return err
		}).Must(context.Background(), hs)
	return view
}

func incrHops(v []*GossipRecord) {
	for _, g := range v {
		g.IncrHop()
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

func getSignedRecord(h host.Host) (*record.Envelope, error) {
	waitReady(h)

	if cab, ok := peerstore.GetCertifiedAddrBook(h.Peerstore()); ok {
		return cab.GetPeerRecord(h.ID()), nil
	}

	return nil, errors.New("no certified addrbook")
}
