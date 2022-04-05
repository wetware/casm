package socket_test

import (
	"testing"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/record"
	inproc "github.com/lthibault/go-libp2p-inproc-transport"
	"github.com/stretchr/testify/require"
	"github.com/wetware/casm/pkg/boot/socket"
)

func TestCache(t *testing.T) {
	t.Parallel()

	h, err := libp2p.New(
		libp2p.NoTransports,
		libp2p.NoListenAddrs,
		libp2p.Transport(inproc.New()),
		libp2p.ListenAddrStrings("/inproc/~"))
	require.NoError(t, err, "must create test host")

	pk := h.Peerstore().PrivKey(h.ID())

	seal := func(r record.Record) (*record.Envelope, error) {
		return record.Seal(r, pk)
	}

	var (
		cache          = socket.NewCache(8)
		req, res, surv *record.Envelope
	)

	// Test LoadRequest by calling it twice.  The first call should generate a new record
	// and the second call should return the cached record.  We do a quick-and-dirty test
	// that asserts these two records are equal.
	t.Run("LoadRequest", func(t *testing.T) {
		// First call to LoadRequest will generate record
		req, err = cache.LoadRequest(seal, h.ID(), "test")
		require.NoError(t, err, "must generate cache entry")

		// Second call to LoadRequest will load record from cache
		req2, err := cache.LoadRequest(seal, h.ID(), "test")
		require.NoError(t, err, "must generate cache entry for request payload")

		require.True(t, req2.Equal(req), "e2 should be equal to e")
	})

	// Now we call LoadResponse and check that the result is different from e and e2.
	// This is a regression test against a bug where all records were stored under
	// the same key.
	t.Run("LoadResponse", func(t *testing.T) {
		t.Run("Succeed", func(t *testing.T) {
			res, err = cache.LoadResponse(seal, h, "test")
			require.NoError(t, err, "must generate cache entry for response payload")
			require.False(t, res.Equal(req), "res should contain a different payload than req")
		})

		t.Run("Fail/NoAddrs", func(t *testing.T) {
			t.Parallel()

			h, err := libp2p.New(
				libp2p.NoTransports,
				libp2p.NoListenAddrs,
				libp2p.Transport(inproc.New()))
			require.NoError(t, err, "must create test host")

			cache := socket.NewCache(8)

			res, err = cache.LoadResponse(seal, h, "test")
			require.ErrorIs(t, err, socket.ErrNoListenAddrs, "must generate cache entry for response payload")
			require.Nil(t, res, "should return nil envelope")
		})
	})

	// Lastly, check that LoadSurveyRequest returns a payload distinct from the first two.
	t.Run("LoadSurveyRequest", func(t *testing.T) {
		surv, err = cache.LoadSurveyRequest(seal, h.ID(), "test", 8)
		require.NoError(t, err, "must generate cache entry for response payload")
		require.False(t, surv.Equal(req), "surv should contain a different payload than req")
	})
}
