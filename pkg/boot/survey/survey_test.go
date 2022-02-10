package survey_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wetware/casm/pkg/boot/survey"
	mx "github.com/wetware/matrix/pkg"
)

const (
	testNs       = "casm/survey"
	advertiseTTL = time.Minute
)

func TestTransport(t *testing.T) {
	t.Parallel()

	var tpt survey.Transport

	// Change port in order to avoid conflicts between parallel tests.
	const multicastAddr = "228.8.8.8:8823"
	addr, err := net.ResolveUDPAddr("udp4", multicastAddr)
	require.NoError(t, err)

	lconn, err := tpt.Listen(addr)
	require.NoError(t, err)
	require.NotNil(t, lconn)
	defer lconn.Close()

	dconn, err := tpt.Dial(addr)
	require.NoError(t, err)
	require.NotNil(t, dconn)
	defer dconn.Close()

	ch := make(chan []byte, 1)
	go func() {
		defer close(ch)
		var buf [4]byte
		n, _, err := lconn.ReadFrom(buf[:])
		require.NoError(t, err)
		ch <- buf[:n]
	}()

	n, err := dconn.WriteTo([]byte("test"), addr)
	require.NoError(t, err)
	assert.Equal(t, len("test"), n)

	b := <-ch
	require.Equal(t, "test", string(b))
}

func TestDiscover(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sim := mx.New(ctx)
	h1 := sim.MustHost(ctx)
	defer h1.Close()
	h2 := sim.MustHost(ctx)
	defer h2.Close()

	// Change port in order to avoid conflicts between parallel tests.
	const multicastAddr = "228.8.8.8:8824"
	addr, _ := net.ResolveUDPAddr("udp4", multicastAddr)

	s1, err := survey.New(h1, addr)
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, s1.Close(), "should close without error")
	}()

	s2, err := survey.New(h2, addr)
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, s2.Close(), "should close without error")
	}()

	ttl, err := s2.Advertise(ctx, testNs, discovery.TTL(advertiseTTL))
	require.NoError(t, err)
	require.Equal(t, advertiseTTL, ttl)

	finder, err := s1.FindPeers(ctx, testNs)
	require.NoError(t, err)

	info := <-finder
	require.Equal(t, info, *host.InfoFromHost(h2))
}

func TestClose(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := mx.New(ctx).MustHost(ctx)
	defer h.Close()

	// Change port in order to avoid conflicts between parallel tests.
	const multicastAddr = "228.8.8.8:8825"
	addr, _ := net.ResolveUDPAddr("udp4", multicastAddr)

	s1, err := survey.New(h, addr)
	require.NoError(t, err)

	ttl, err := s1.Advertise(ctx, testNs, discovery.TTL(advertiseTTL))
	require.NoError(t, err)
	require.Equal(t, advertiseTTL, ttl)

	err = s1.Close()
	require.NoError(t, err)

	ttl, err = s1.Advertise(ctx, testNs, discovery.TTL(advertiseTTL))
	require.Error(t, err, "closed")
	require.Zero(t, ttl)
}
