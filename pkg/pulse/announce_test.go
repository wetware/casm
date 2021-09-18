package pulse

import (
	"testing"

	"capnproto.org/go/capnp/v3"
	"github.com/stretchr/testify/require"
)

func TestAnnouncement_MarshalUnmarshal(t *testing.T) {
	t.Parallel()

	a, err := newAnnouncement(capnp.SingleSegment(nil))
	require.NoError(t, err)

	hb, err := a.NewHeartbeat()
	require.NoError(t, err)
	hb.SetTTL(42)

	want, err := capnp.Canonicalize(a.Struct)
	require.NoError(t, err)

	// marshal
	b, err := a.MarshalBinary()
	require.NoError(t, err)
	require.NotEmpty(t, b)

	// unmarshal
	a = announcement{}
	err = a.UnmarshalBinary(b)
	require.NoError(t, err)

	got, err := capnp.Canonicalize(a.Struct)
	require.NoError(t, err)

	// validate
	require.Equal(t, want, got)
}
