package pulse_test

import (
	"testing"

	"capnproto.org/go/capnp/v3"
	"github.com/stretchr/testify/require"
	"github.com/wetware/casm/pkg/cluster/pulse"
)

func TestClusterEvent_MarshalUnmarshal(t *testing.T) {
	t.Parallel()

	ev, err := pulse.NewClusterEvent(capnp.SingleSegment(nil))
	require.NoError(t, err)

	hb, err := ev.NewHeartbeat()
	require.NoError(t, err)
	hb.SetTTL(42)

	// marshal
	b, err := ev.MarshalBinary()
	require.NoError(t, err)
	require.NotEmpty(t, b)

	// unmarshal
	ev2 := pulse.ClusterEvent{}
	err = ev2.UnmarshalBinary(b)
	require.NoError(t, err)

	// TODO:  check that entries are valid
}
