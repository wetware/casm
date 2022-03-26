package crawl_test

import (
	"testing"

	ma "github.com/multiformats/go-multiaddr"

	"github.com/wetware/casm/pkg/boot/crawl"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStrategy(t *testing.T) {
	t.Parallel()
	t.Helper()

	t.Run("CIDR", func(t *testing.T) {
		t.Parallel()

		addr := ma.StringCast("/ip4/228.8.8.8/udp/8822/cidr/24")

		cidr, err := crawl.ParseCIDR(addr)
		require.NoError(t, err, "should succeed")
		require.NotNil(t, cidr, "should return strategy")

		c, err := cidr()
		assert.NoError(t, err, "should succeed")
		assert.IsType(t, new(crawl.CIDR), c, "should return CIDR range")
	})
}
