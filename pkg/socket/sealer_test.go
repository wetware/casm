package socket_test

import (
	"errors"
	"math/rand"
	"testing"

	"github.com/cespare/xxhash/v2"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/record"

	"github.com/wetware/casm/pkg/socket"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCachingSealer(t *testing.T) {
	t.Parallel()

	pk, _, err := crypto.GenerateEd25519Key(rand.New(rand.NewSource(42)))
	require.NoError(t, err)

	r := mockRecord("testing")

	t.Run("BasicRecord", func(t *testing.T) {
		t.Parallel()

		sealer, err := socket.NewCachingSealer(pk, 8)
		require.NoError(t, err)
		require.NotNil(t, sealer)
		defer sealer.Purge()

		want, err := sealer.Seal(&r)
		t.Logf("[COMPUTED %d bytes] %s", len(want), want)
		require.NoError(t, err)
		require.NotNil(t, want)

		got, err := sealer.Seal(&r)
		t.Logf("[FETCHED %d bytes] %s", len(got), got)
		require.NoError(t, err)
		require.Equal(t, want, got)
	})

	t.Run("Hashable", func(t *testing.T) {
		t.Parallel()

		sealer, err := socket.NewCachingSealer(pk, 8)
		require.NoError(t, err)
		require.NotNil(t, sealer)
		defer sealer.Purge()

		want, err := sealer.Seal(&r)
		t.Logf("[COMPUTED %d bytes] %s", len(want), want)
		require.NoError(t, err)
		require.NotNil(t, want)

		got, err := sealer.Seal(mockHashable{&r})
		t.Logf("[FETCHED %d bytes] %s", len(got), got)
		require.NoError(t, err)
		require.Equal(t, want, got)
	})

	t.Run("RecordError", func(t *testing.T) {
		t.Parallel()

		sealer, err := socket.NewCachingSealer(pk, 8)
		require.NoError(t, err)
		require.NotNil(t, sealer)
		defer sealer.Purge()

		got, err := sealer.Seal(mockErrorRecord{errors.New("test")})
		assert.Nil(t, got, "should not return payload")
		assert.EqualError(t, err, "test")
	})
}

func BenchmarkCachingSealer(b *testing.B) {
	pk, _, err := crypto.GenerateEd25519Key(rand.New(rand.NewSource(42)))
	require.NoError(b, err)

	sealer, err := socket.NewCachingSealer(pk, 8)
	require.NoError(b, err)
	require.NotNil(b, sealer)

	r := mockRecord("testing")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = sealer.Seal(&r)
	}
}

var _ record.Record = (*mockRecord)(nil)

type mockRecord string

func (r mockRecord) Domain() string {
	return string(r)
}

func (mockRecord) Codec() []byte {
	return []byte{0xFF, 0xFF}
}

func (r mockRecord) MarshalRecord() ([]byte, error) {
	return []byte(r), nil
}

func (r *mockRecord) UnmarshalRecord(b []byte) error {
	*r = mockRecord(b)
	return nil
}

type mockErrorRecord struct {
	error
}

func (r mockErrorRecord) Domain() string {
	return r.Error()
}

func (mockErrorRecord) Codec() []byte {
	return []byte{0xFF, 0xFF}
}

func (r mockErrorRecord) MarshalRecord() ([]byte, error) {
	return nil, r.error
}

func (r mockErrorRecord) UnmarshalRecord([]byte) error {
	return r.error
}

type mockHashable struct{ record.Record }

func (h mockHashable) Hash() (uint64, error) {
	b, err := h.MarshalRecord()
	return xxhash.Sum64(b), err
}
