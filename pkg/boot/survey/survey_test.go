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
	testNs        = "casm/survey"
	advertiseTTL  = time.Minute
	multicastAddr = "224.0.1.241:3037"
)

func TestTransport(t *testing.T) {
	t.Parallel()

	var tpt survey.Transport

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

	addr, _ := net.ResolveUDPAddr("udp4", multicastAddr)

	a1, err := survey.New(h1, addr)
	require.NoError(t, err)
	defer a1.Close()

	a2, err := survey.New(h2, addr)
	require.NoError(t, err)
	defer a2.Close()

	a1.Advertise(ctx, testNs, discovery.TTL(advertiseTTL))
	a2.Advertise(ctx, testNs, discovery.TTL(advertiseTTL))

	finder, err := a1.FindPeers(ctx, testNs)
	require.NoError(t, err)

	info := <-finder
	require.Equal(t, info, *host.InfoFromHost(h2))
}

func TestClose(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sim := mx.New(ctx)
	h1 := sim.MustHost(ctx)
	defer h1.Close()

	addr, _ := net.ResolveUDPAddr("udp4", multicastAddr)

	a1, err := survey.New(h1, addr)
	require.NoError(t, err)

	_, err = a1.Advertise(ctx, testNs, discovery.TTL(advertiseTTL))
	require.NoError(t, err)

	a1.Close()

	_, err = a1.Advertise(ctx, testNs, discovery.TTL(advertiseTTL))
	require.Error(t, err, "closed")
}
