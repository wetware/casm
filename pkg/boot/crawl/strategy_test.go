package crawl_test

import (
	"net"
	"net/netip"
	"testing"

	ma "github.com/multiformats/go-multiaddr"

	"github.com/wetware/casm/pkg/boot/crawl"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCIDR(t *testing.T) {
	t.Parallel()

	maddr := ma.StringCast("/ip4/228.8.8.8/udp/8822/cidr/24")

	cidr, err := crawl.ParseCIDR(maddr)
	require.NoError(t, err, "should succeed")
	require.NotNil(t, cidr, "should return strategy")

	c, err := cidr()
	assert.NoError(t, err, "should succeed")
	assert.IsType(t, new(crawl.CIDR), c, "should return CIDR range")

	seen := map[netip.Addr]struct{}{}

	var addr net.UDPAddr
	for c.Next(&addr) {
		ip, ok := netip.AddrFromSlice(addr.IP)
		require.True(t, ok, "%s is not a valid IP address", addr.IP)
		assert.NotContains(t, seen, ip, "duplicate address: %s", ip)

		seen[ip] = struct{}{}
	}

	assert.Len(t, seen, 254,
		"should contain 8-bit subnet without X.X.X.0 and X.X.X.255")
}
