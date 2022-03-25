package boot

import (
	"testing"

	ma "github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wetware/casm/pkg/boot/crawl"
)

func TestParse(t *testing.T) {
	t.Parallel()
	t.Helper()

	for _, tt := range []struct {
		name, addr string
		test       func(ma.Multiaddr) bool
	}{
		{
			name: "crawler",
			addr: "/ip4/228.8.8.8/udp/8822/cidr/24",
			test: crawler,
		},
		{
			name: "multicast",
			addr: "/ip4/228.8.8.8/udp/8820/multicast/lo0",
			test: multicast,
		},
		{
			name: "multicast/survey",
			addr: "/ip4/228.8.8.8/udp/8822/multicast/lo0/survey",
			test: gradual,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.True(t, tt.test(ma.StringCast(tt.addr)),
				"should match")
		})
	}
}

func TestStrategy(t *testing.T) {
	t.Parallel()

	addr := ma.StringCast("/ip4/228.8.8.8/udp/8822/cidr/24")
	s, err := strategy(addr)
	require.NoError(t, err, "should succeed")
	assert.IsType(t, new(crawl.CIDR), s(),
		"should produce CIDR-crawl strategy")
}
