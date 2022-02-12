package boot_test

import (
	"context"
	"testing"

	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wetware/casm/pkg/boot"
	"github.com/wetware/casm/pkg/boot/survey"
	mx "github.com/wetware/matrix/pkg"
)

func TestMultiaddr(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		addr string
		fail bool
	}{
		{"/ip4/228.8.8.8/udp/8822/survey", false},
		{"/ip4/228.8.8.8/udp/8822/cidr/32", false},
		{"/ip4/228.8.8.8/udp/8822/survey/gradual", false},
		{"/ip4/228.8.8.8/udp/8822/cidr/129", true},
	} {
		_, err := multiaddr.NewMultiaddr(tt.addr)
		if tt.fail {
			assert.Error(t, err, "should fail to parse %s", tt.addr)
		} else {
			assert.NoError(t, err, "should parse %s", tt.addr)
		}
	}
}

func TestParseLayer4(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		addr string
		fail bool
	}{
		{"/ip4/228.8.8.8/udp/8822/cidr/32", false},
		{"/ip4/228.8.8.8/udp/8822/survey", false},
		{"/ip4/228.8.8.8/udp/8822/survey/gradual", false},
		{"/ip4/228.8.8.8", true},
		{"/udp/8822/ip4/228.8.8.8", true},
		{"/ip4/228.8.8.8/unix/foo", true},
		{"/unix/foo", true},
	} {
		_, err := boot.ParseLayer4(multiaddr.StringCast(tt.addr))
		if tt.fail {
			assert.Error(t, err, "should fail to parse %s", tt.addr)
		} else {
			assert.NoError(t, err, "should parse %s", tt.addr)
		}
	}
}

func TestTranscoderCIDR(t *testing.T) {
	t.Parallel()
	t.Helper()

	t.Run("StringToBytes", func(t *testing.T) {
		t.Parallel()

		s, err := boot.TranscoderCIDR{}.BytesToString([]byte{0x00, 0x00})
		assert.ErrorIs(t, err, boot.ErrCIDROverflow,
			"should not parse byte arrays of length > 1")
		assert.Empty(t, s)

		s, err = boot.TranscoderCIDR{}.BytesToString([]byte{0xFF})
		assert.ErrorIs(t, err, boot.ErrCIDROverflow,
			"should not validate CIDR greater than 128")
		assert.Empty(t, s)

		s, err = boot.TranscoderCIDR{}.BytesToString([]byte{0x01})
		assert.NoError(t, err, "should parse CIDR of 1")
		assert.Equal(t, "1", s, "should return \"1\"")
	})

	t.Run("BytesToString", func(t *testing.T) {
		t.Parallel()

		b, err := boot.TranscoderCIDR{}.StringToBytes("fail")
		assert.Error(t, err,
			"should not validate non-numerical strings")
		assert.Nil(t, b)

		b, err = boot.TranscoderCIDR{}.StringToBytes("255")
		assert.ErrorIs(t, err, boot.ErrCIDROverflow,
			"should not validate string '255'")
		assert.Nil(t, b)
	})

	t.Run("ValidateBytes", func(t *testing.T) {
		t.Parallel()

		err := boot.TranscoderCIDR{}.ValidateBytes([]byte{0x00})
		assert.NoError(t, err,
			"should validate CIDR block of 0")

		err = boot.TranscoderCIDR{}.ValidateBytes([]byte{0xFF})
		assert.ErrorIs(t, err, boot.ErrCIDROverflow,
			"should not validate CIDR blocks greater than 128")
	})
}

func TestParse(t *testing.T) {
	t.Parallel()
	t.Helper()

	t.Run("Survey", func(t *testing.T) {
		t.Parallel()
		t.Helper()

		t.Run("Surveyor", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			sim := mx.New(ctx)
			h := sim.MustHost(ctx)
			defer h.Close()

			maddr := multiaddr.StringCast("/ip4/228.8.8.8/udp/8820/survey")
			d, err := boot.Parse(h, maddr)
			require.NoError(t, err)

			assert.IsType(t, new(survey.Surveyor), d,
				"should be standard surveyor")
		})

		t.Run("GradualSurveyor", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			sim := mx.New(ctx)
			h := sim.MustHost(ctx)
			defer h.Close()

			maddr := multiaddr.StringCast("/ip4/228.8.8.8/udp/8820/survey/gradual")
			d, err := boot.Parse(h, maddr)
			require.NoError(t, err)

			assert.IsType(t, survey.GradualSurveyor{}, d,
				"should be gradual surveyor")
		})
	})

	t.Run("Crawl", func(t *testing.T) {
		t.Parallel()
		t.Helper()

		t.Run("Succeed", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			sim := mx.New(ctx)
			h := sim.MustHost(ctx)
			defer h.Close()

			maddr := multiaddr.StringCast("/ip4/228.8.8.8/tcp/8820/cidr/24")
			disc, err := boot.Parse(h, maddr)
			require.NoError(t, err)

			_, ok := disc.(boot.Crawler)
			require.True(t, ok)
		})

		t.Run("Invalid", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			sim := mx.New(ctx)
			h := sim.MustHost(ctx)
			defer h.Close()

			require.Panics(t, func() {
				multiaddr.StringCast("/ip4/228.8.8.8/udp/8820/cidr/200")
			})

			require.Panics(t, func() {
				multiaddr.StringCast("/ip4/228.8.8.8/udp/8820/cidr/-1")
			})
		})
	})
}
