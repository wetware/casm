package pulse_test

import (
	"testing"
	"time"

	"capnproto.org/go/capnp/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wetware/casm/pkg/cluster/pulse"
)

func TestHeartbeat_MarshalUnmarshal(t *testing.T) {
	t.Parallel()

	hb, err := pulse.NewHeartbeat(capnp.SingleSegment(nil))
	require.NoError(t, err)

	hb.SetTTL(42)
	require.Equal(t, time.Duration(42), hb.TTL())

	// marshal
	b, err := hb.MarshalBinary()
	require.NoError(t, err)
	require.NotEmpty(t, b)

	// unmarshal
	hb2 := pulse.Heartbeat{}
	err = hb2.UnmarshalBinary(b)
	require.NoError(t, err)

	assert.Equal(t, hb.TTL(), hb2.TTL())
}
