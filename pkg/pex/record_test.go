package pex_test

import (
	"bytes"
	"crypto/rand"
	"testing"

	"capnproto.org/go/capnp/v3"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/record"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
	"github.com/wetware/casm/pkg/pex"
)

func TestNewGossipRecord(t *testing.T) {
	t.Parallel()

	_, e := newTestRecord()

	g, err := pex.NewGossipRecord(e)
	require.NoError(t, err)

	require.Zero(t, g.Hop())
	g.IncrHop()
	require.Equal(t, uint64(1), g.Hop())
}

func TestGossipRecord_MarshalUnmarshal(t *testing.T) {
	t.Parallel()

	_, e := newTestRecord()

	want, err := pex.NewGossipRecord(e)
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

	var got pex.GossipRecord
	err = got.ReadMessage(msg)
	require.NoError(t, err)

	// validate
	require.Equal(t, want.Hop(), got.Hop())
	require.Equal(t, want.PeerID, got.PeerID)

	require.NotNil(t, got.Addrs)
	require.Len(t, got.Addrs, len(want.Addrs))
	for i, expect := range want.Addrs {
		require.True(t, expect.Equal(got.Addrs[i]))
	}
}

// func TestView_validation(t *testing.T) {
// 	t.Parallel()
// 	t.Helper()

// 	t.Run("Succeed", func(t *testing.T) {
// 		t.Parallel()

// 		ctx, cancel := context.WithCancel(context.Background())
// 		defer cancel()

// 		sim := mx.New(ctx)

// 		hs := sim.MustHostSet(ctx, 2)
// 		gs := mustGossipSlice(hs)

// 		// records from peers other than the sender MUST have hop > 0 in
// 		// order to pass validation.
// 		incrHops(gs[:len(gs)-1])

// 		err := gs.Validate()
// 		require.NoError(t, err)
// 	})

// 	t.Run("SenderOnly", func(t *testing.T) {
// 		t.Parallel()

// 		ctx, cancel := context.WithCancel(context.Background())
// 		defer cancel()

// 		sim := mx.New(ctx)

// 		hs := sim.MustHostSet(ctx, 1)
// 		gs := mustGossipSlice(hs)

// 		/*
// 		 * There is only one record (from the sender), so don't
// 		 * increment hops.  This should pass.
// 		 */

// 		err := gs.Validate()
// 		require.NoError(t, err)
// 	})

// 	t.Run("Fail", func(t *testing.T) {
// 		t.Helper()
// 		t.Parallel()

// 		t.Run("empty", func(t *testing.T) {
// 			t.Parallel()

// 			var gs gossipSlice
// 			err := gs.Validate()
// 			require.ErrorAs(t, err, &ValidationError{})
// 			require.EqualError(t, err, "empty view")
// 		})

// 		t.Run("sender_invalid_hop_range", func(t *testing.T) {
// 			t.Parallel()

// 			ctx, cancel := context.WithCancel(context.Background())
// 			defer cancel()

// 			sim := mx.New(ctx)

// 			hs := sim.MustHostSet(ctx, 2)
// 			gs := mustGossipSlice(hs)

// 			/*
// 			 * N.B.:  we increment hops for _all_ records, including the
// 			 *        sender.  This should be caught in validation.
// 			 */
// 			incrHops(gs)

// 			err := gs.Validate()
// 			require.ErrorAs(t, err, &ValidationError{})
// 			require.ErrorIs(t, err, ErrInvalidRange)
// 		})

// 		t.Run("invalid_hop_range", func(t *testing.T) {
// 			t.Parallel()

// 			ctx, cancel := context.WithCancel(context.Background())
// 			defer cancel()

// 			sim := mx.New(ctx)

// 			hs := sim.MustHostSet(ctx, 2)

// 			/*
// 			 * N.B.:  we don't increment the hops here; this should be
// 			 *        caught in validation.
// 			 */

// 			err := mustGossipSlice(hs).Validate()
// 			require.ErrorAs(t, err, &ValidationError{})
// 			require.ErrorIs(t, err, ErrInvalidRange)
// 		})
// 	})
// }

func newTestRecord() (*peer.PeerRecord, *record.Envelope) {
	priv, pub, err := crypto.GenerateECDSAKeyPair(rand.Reader)
	if err != nil {
		panic(err)
	}

	id, err := peer.IDFromPublicKey(pub)
	if err != nil {
		panic(err)
	}

	var rec = &peer.PeerRecord{
		PeerID: id,
		Addrs:  []ma.Multiaddr{ma.StringCast("/ip4/127.0.0.1/tcp/92")},
		Seq:    1,
	}

	e, err := record.Seal(rec, priv)
	if err != nil {
		panic(err)
	}

	return rec, e
}

// func mustGossipSlice(hs []host.Host) gossipSlice {
// 	gs := make([]*GossipRecord, len(hs))
// 	mx.
// 		Go(func(ctx context.Context, i int, h host.Host) error {
// 			waitReady(h)
// 			return nil
// 		}).
// 		Go(func(ctx context.Context, i int, h host.Host) error {
// 			e, err := getSignedRecord(h)
// 			if err == nil {
// 				gs[i], err = NewGossipRecord(e)
// 			}

// 			return err
// 		}).Must(context.Background(), hs)
// 	return gs
// }

// func incrHops(gs []*GossipRecord) {
// 	for _, g := range gs {
// 		g.IncrHop()
// 	}
// }

// func waitReady(h host.Host) {
// 	sub, err := h.EventBus().Subscribe(new(event.EvtLocalAddressesUpdated))
// 	if err != nil {
// 		panic(err)
// 	}
// 	defer sub.Close()

// 	<-sub.Out()
// }

// func getSignedRecord(h host.Host) (*record.Envelope, error) {
// 	waitReady(h)

// 	if cab, ok := peerstore.GetCertifiedAddrBook(h.Peerstore()); ok {
// 		return cab.GetPeerRecord(h.ID()), nil
// 	}

// 	return nil, errors.New("no certified addrbook")
// }
