package crawl_test

import (
	"net"
	"testing"

	ma "github.com/multiformats/go-multiaddr"

	"github.com/wetware/casm/pkg/boot/crawl"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCIDR(t *testing.T) {
	t.Parallel()
	t.Helper()

	t.Run("IPv4", func(t *testing.T) {
		t.Parallel()
		t.Helper()

		t.Run("ByteAligned", func(t *testing.T) {
			t.Parallel()

			maddr := ma.StringCast("/ip4/228.8.8.8/udp/8822/cidr/24")

			cidr, err := crawl.ParseCIDR(maddr)
			require.NoError(t, err, "should succeed")
			require.NotNil(t, cidr, "should return strategy")

			c, err := cidr()
			assert.NoError(t, err, "should succeed")
			assert.IsType(t, new(crawl.CIDR), c, "should return CIDR range")

			seen := map[string]struct{}{}

			var addr net.UDPAddr
			for c.Next(&addr) {
				seen[addr.String()] = struct{}{}
			}

			assert.Len(t, seen, 254,
				"should contain 8-bit subnet without network & broadcast addrs")
		})


		t.Run("Unaligned", func(t *testing.T) {
			t.Parallel()

			maddr := ma.StringCast("/ip4/228.8.8.8/udp/8822/cidr/21")

			cidr, err := crawl.ParseCIDR(maddr)
			require.NoError(t, err, "should succeed")
			require.NotNil(t, cidr, "should return strategy")

			c, err := cidr()
			assert.NoError(t, err, "should succeed")
			assert.IsType(t, new(crawl.CIDR), c, "should return CIDR range")

			seen := map[string]struct{}{}

			var addr net.UDPAddr
			for c.Next(&addr) {
				seen[addr.String()] = struct{}{}
			}

			assert.Len(t, seen, 2046,
				"should contain 6-bit subnet without network & broadcast addrs")
		})

		t.Run("/32", func(t *testing.T) {
			t.Parallel()

			maddr := ma.StringCast("/ip4/228.8.8.8/udp/8822/cidr/32")

			cidr, err := crawl.ParseCIDR(maddr)
			require.NoError(t, err, "should succeed")
			require.NotNil(t, cidr, "should return strategy")

			c, err := cidr()
			assert.NoError(t, err, "should succeed")
			assert.IsType(t, new(crawl.CIDR), c, "should return CIDR range")

			seen := map[string]struct{}{}

			var addr net.UDPAddr
			for c.Next(&addr) {
				seen[addr.String()] = struct{}{}
			}

			assert.Len(t, seen, 1,
				"should contain 1 subnet")
		})
	})

	t.Run("IPv6", func(t *testing.T) {
		t.Parallel()
		t.Helper()

		t.Run("ByteAligned", func(t *testing.T) {
			t.Parallel()

			maddr := ma.StringCast("/ip6/2001:db8::/udp/8822/cidr/120")

			cidr, err := crawl.ParseCIDR(maddr)
			require.NoError(t, err, "should succeed")
			require.NotNil(t, cidr, "should return strategy")

			c, err := cidr()
			assert.NoError(t, err, "should succeed")
			assert.IsType(t, new(crawl.CIDR), c, "should return CIDR range")

			seen := map[string]struct{}{}

			var addr net.UDPAddr
			for c.Next(&addr) {
				seen[addr.String()] = struct{}{}
			}

			assert.Len(t, seen, 254,
				"should contain 8-bit subnet without network & broadcast addrs")
		})

		t.Run("Unaligned", func(t *testing.T) {
			t.Parallel()

			maddr := ma.StringCast("/ip6/2001:db8::/udp/8822/cidr/117")

			cidr, err := crawl.ParseCIDR(maddr)
			require.NoError(t, err, "should succeed")
			require.NotNil(t, cidr, "should return strategy")

			c, err := cidr()
			assert.NoError(t, err, "should succeed")
			assert.IsType(t, new(crawl.CIDR), c, "should return CIDR range")

			seen := map[string]struct{}{}

			var addr net.UDPAddr
			for c.Next(&addr) {
				seen[addr.String()] = struct{}{}
			}

			t.Log(len(seen))
			assert.Len(t, seen, 2046,
				"should contain 6-bit subnet without network & broadcast addrs")
		})
	})
}

func BenchmarkCIDR(b *testing.B) {
	for _, bt := range []struct {
		name string
		CIDR ma.Multiaddr
	}{
		{
			name: "24",
			CIDR: ma.StringCast("/ip4/228.8.8.8/udp/8822/cidr/24"),
		},
		{
			name: "16",
			CIDR: ma.StringCast("/ip4/228.8.8.8/udp/8822/cidr/16"),
		},
		{
			name: "8",
			CIDR: ma.StringCast("/ip4/228.8.8.8/udp/8822/cidr/8"),
		},
	} {
		b.Run(bt.name, func(b *testing.B) {
			cidr, err := crawl.ParseCIDR(bt.CIDR)
			if err != nil {
				panic(err)
			}

			c, err := cidr()
			if err != nil {
				panic(err)
			}

			b.ResetTimer()

			var addr net.UDPAddr
			for i := 0; i < b.N; i++ {
				for c.Next(&addr) {
					// iterate...
				}
			}
		})
	}
}
