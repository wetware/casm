package pex_test

import (
	"bytes"
	"crypto/rand"
	"io"
	"testing"

	"capnproto.org/go/capnp/v3"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/record"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/pierrec/lz4/v4"
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

	logPayloadSize(t, &buf)

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

func logPayloadSize(t *testing.T, buf *bytes.Buffer) {
	t.Logf("uncompressed:  %d bytes", buf.Len())

	var c lz4.Compressor
	wbuf := make([]byte, lz4.CompressBlockBound(buf.Len()))

	n, err := c.CompressBlock(buf.Bytes(), wbuf)
	require.NoError(t, err)

	t.Logf("lz4-compressed: %d bytes (diff=%.0f%%)", n,
		(1.-float64(n)/float64(buf.Len()))*100.)
}

func BenchmarkGossipRecord_MarshalUnmarshal(b *testing.B) {
	var (
		buf    = bytes.NewBuffer(make([]byte, 0, 1024))
		_, env = newTestRecord()
		rec, _ = pex.NewGossipRecord(env)
		m      *capnp.Message
	)

	enc := capnp.NewEncoder(buf)
	b.Run("Write", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			buf.Reset()
			_ = enc.Encode(rec.Message())
		}
	})

	rd := bytes.NewReader(buf.Bytes())
	dec := capnp.NewDecoder(rd)
	b.Run("Read", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			m, _ = dec.Decode()
			rd.Seek(0, io.SeekStart)
		}
	})

	b.Run("ReadMessage", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = rec.ReadMessage(m)
		}
	})
}

func TestView_validation(t *testing.T) {
	t.Parallel()
	t.Helper()

	t.Run("Succeed", func(t *testing.T) {
		t.Parallel()

		hs := []host.Host{newTestHost(), newTestHost()}
		v := mustView(hs)

		// records from peers other than the sender MUST have hop > 0 in
		// order to pass validation.
		incrHops(v[:len(v)-1])

		err := v.Validate()
		require.NoError(t, err)
	})

	t.Run("SenderOnly", func(t *testing.T) {
		t.Parallel()

		hs := []host.Host{newTestHost()}
		v := mustView(hs)

		/*
		 * There is only one record (from the sender), so don't
		 * increment hops.  This should pass.
		 */

		err := v.Validate()
		require.NoError(t, err)
	})

	t.Run("Fail", func(t *testing.T) {
		t.Helper()
		t.Parallel()

		t.Run("empty", func(t *testing.T) {
			t.Parallel()

			var v pex.View
			err := v.Validate()
			require.ErrorAs(t, err, &pex.ValidationError{})
			require.EqualError(t, err, "empty view")
		})

		t.Run("sender_invalid_hop_range", func(t *testing.T) {
			t.Parallel()

			hs := []host.Host{newTestHost(), newTestHost()}
			v := mustView(hs)

			/*
			 * N.B.:  we increment hops for _all_ records, including the
			 *        sender.  This should be caught in validation.
			 */
			incrHops(v)

			err := v.Validate()
			require.ErrorAs(t, err, &pex.ValidationError{})
			require.ErrorIs(t, err, pex.ErrInvalidRange)
		})

		t.Run("invalid_hop_range", func(t *testing.T) {
			t.Parallel()

			hs := []host.Host{newTestHost(), newTestHost()}

			/*
			 * N.B.:  we don't increment the hops here; this should be
			 *        caught in validation.
			 */

			err := mustView(hs).Validate()
			require.ErrorAs(t, err, &pex.ValidationError{})
			require.ErrorIs(t, err, pex.ErrInvalidRange)
		})
	})
}

func newTestRecord() (*peer.PeerRecord, *record.Envelope) {
	priv, pub, err := crypto.GenerateEd25519Key(rand.Reader)
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

func mustView(hs []host.Host) pex.View {
	var err error
	v := make([]*pex.GossipRecord, len(hs))
	for i, h := range hs {
		if v[i], err = pex.NewGossipRecord(waitReady(h)); err != nil {
			panic(err)
		}

	}
	return v
}

func incrHops(v []*pex.GossipRecord) {
	for _, g := range v {
		g.IncrHop()
	}
}

func waitReady(h host.Host) *record.Envelope {
	sub, err := h.EventBus().Subscribe(new(event.EvtLocalAddressesUpdated))
	if err != nil {
		panic(err)
	}
	defer sub.Close()

	v := <-sub.Out()
	return v.(event.EvtLocalAddressesUpdated).SignedPeerRecord
}
