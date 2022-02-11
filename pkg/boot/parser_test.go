package boot

import (
	"context"
	"testing"

	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
	"github.com/wetware/casm/pkg/boot/survey"
	mx "github.com/wetware/matrix/pkg"
)

func TestParseSurvey(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sim := mx.New(ctx)
	h := sim.MustHost(ctx)
	defer h.Close()

	maddr := multiaddr.StringCast("/ip4/228.8.8.8/udp/8820/survey")
	disc, err := Parse(h, maddr)
	require.NoError(t, err)

	_, ok := disc.(*survey.Surveyor)
	require.True(t, ok)
}

func TestParseGradualSurveyor(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sim := mx.New(ctx)
	h := sim.MustHost(ctx)
	defer h.Close()

	maddr := multiaddr.StringCast("/ip4/228.8.8.8/udp/8820/survey/gradual")
	disc, err := Parse(h, maddr)
	require.NoError(t, err)

	_, ok := disc.(*survey.GradualSurveyor)
	require.True(t, ok)
}

func TestParseCrawler(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sim := mx.New(ctx)
	h := sim.MustHost(ctx)
	defer h.Close()

	maddr := multiaddr.StringCast("/ip4/228.8.8.8/udp/8820/crawl/24")
	disc, err := Parse(h, maddr)
	require.NoError(t, err)

	_, ok := disc.(Crawler)
	require.True(t, ok)
}

func TestParseInvalidCrawler(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sim := mx.New(ctx)
	h := sim.MustHost(ctx)
	defer h.Close()

	require.Panics(t, func() {
		multiaddr.StringCast("/ip4/228.8.8.8/udp/8820/crawl/200")
	})

	require.Panics(t, func() {
		multiaddr.StringCast("/ip4/228.8.8.8/udp/8820/crawl/-1")
	})
}
