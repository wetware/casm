package multicast_test

import (
	"context"
	"testing"

	"capnproto.org/go/capnp/v3"
	"github.com/stretchr/testify/require"
	"github.com/wetware/casm/internal/api/boot"
	"github.com/wetware/casm/pkg/boot/multicast"
)

func TestUDPConn(t *testing.T) {
	t.Parallel()

	conn, err := multicast.NewUDPConn(context.Background(), "228.8.8.8:8822", nil)
	require.NoError(t, err)

	t.Run("Gather", func(t *testing.T) {
		t.Parallel()

		msg, err := conn.Gather(context.Background())
		require.NoError(t, err)

		pkt, err := boot.ReadRootMulticastPacket(msg)
		require.NoError(t, err)

		s, err := pkt.Query()
		require.NoError(t, err)
		require.Equal(t, "test", s)
	})

	t.Run("Scatter", func(t *testing.T) {
		t.Parallel()

		msg, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
		require.NoError(t, err)

		pkt, err := boot.NewRootMulticastPacket(seg)
		require.NoError(t, err)

		err = pkt.SetQuery("test")
		require.NoError(t, err)

		err = conn.Scatter(context.Background(), msg)
		require.NoError(t, err)
	})
}
