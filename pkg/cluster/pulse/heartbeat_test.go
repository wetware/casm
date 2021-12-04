package pulse_test

import (
	"testing"

	"capnproto.org/go/capnp/v3"
	"github.com/stretchr/testify/require"
	"github.com/wetware/casm/pkg/cluster/pulse"
)

func TestHeartbeat_MarshalUnmarshal(t *testing.T) {
	t.Parallel()

	hb, err := pulse.NewHeartbeat(capnp.SingleSegment(nil))
	require.NoError(t, err)
	hb.SetTTL(42)

	// marshal
	b, err := hb.MarshalBinary()
	require.NoError(t, err)
	require.NotEmpty(t, b)

	// unmarshal
	ev2 := pulse.Heartbeat{}
	err = ev2.UnmarshalBinary(b)
	require.NoError(t, err)

	// TODO:  check that entries are valid
}
