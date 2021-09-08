package boot

import (
	"testing"

	ma "github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
)

func TestResolveMulticastAddr(t *testing.T) {
	t.Parallel()

	const addr = "/multicast/ip4/228.8.8.8/udp/8822"

	m, err := resolveMulticastAddr(ma.StringCast(addr))
	require.NoError(t, err)
	require.True(t, m.Equal(ma.StringCast("/ip4/228.8.8.8/udp/8822")))
}
