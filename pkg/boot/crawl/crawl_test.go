package crawl_test

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/host"
	inproc "github.com/lthibault/go-libp2p-inproc-transport"
	"github.com/stretchr/testify/require"
	"github.com/wetware/casm/pkg/boot/crawl"
)

func TestOne(t *testing.T) {
	t.Parallel()
	t.Helper()

	var (
		ip          = net.ParseIP("127.0.1.10")
		cidr string = fmt.Sprintf("%v/24", ip.String())
		port        = 8822
		addr        = &net.UDPAddr{IP: ip, Port: port}
		ns   string = "one"
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := newTestHost()

	iter, err := crawl.NewCIDR(cidr, port)
	require.NoError(t, err)
	require.NotNil(t, iter)

	c, err := crawl.New(h, addr, iter)
	require.Nil(t, err)
	require.NotNil(t, c)
	defer c.Close()

	require.NotNil(t, c)
	finder, err := c.FindPeers(ctx, ns)
	require.Nil(t, err)
	require.NotNil(t, finder)

	n := 0
	for range finder {
		n++
	}
	require.Equal(t, 0, n)
}

func TestTwo(t *testing.T) {
	t.Parallel()
	t.Helper()

	var (
		ip0          = net.ParseIP("127.0.2.10")
		ip1          = net.ParseIP("127.0.2.11")
		port         = 8822
		addr0        = &net.UDPAddr{IP: ip0, Port: port}
		addr1        = &net.UDPAddr{IP: ip1, Port: port}
		ns    string = "two"
		ttl          = time.Hour
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h0 := newTestHost()
	h1 := newTestHost()

	iter0, err := crawl.NewCIDR("127.0.2.0/24", port)
	require.NoError(t, err)

	c0, err := crawl.New(h0, addr0, iter0)
	require.Nil(t, err)
	require.NotNil(t, c0)
	defer c0.Close()

	iter1, err := crawl.NewCIDR("127.0.2.0/24", port)
	require.NoError(t, err)

	c1, err := crawl.New(h1, addr1, iter1)
	require.Nil(t, err)
	require.NotNil(t, c0)
	defer c1.Close()

	_, err = c1.Advertise(ctx, ns, discovery.TTL(ttl))
	require.Nil(t, err)

	finder, err := c0.FindPeers(ctx, ns)
	require.Nil(t, err)
	require.NotNil(t, finder)

	n := 0
	for info := range finder {
		require.EqualValues(t, h1.ID(), info.ID)
		n++
	}
	require.Equal(t, 1, n)
}

func TestMultiple(t *testing.T) {
	t.Parallel()
	t.Helper()

	const (
		N           = 20
		port        = 8822
		ns   string = "multiple"
		ttl         = time.Hour
	)

	var (
		hs  = make([]host.Host, N)
		cs  = make([]*crawl.Crawler, N)
		err error
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for i := 0; i < N; i++ {
		hs[i] = newTestHost()
		defer hs[i].Close()

		iter, err := crawl.NewCIDR("127.0.3.0/24", port)
		require.NoError(t, err)

		cs[i], err = crawl.New(hs[i], &net.UDPAddr{IP: net.ParseIP(fmt.Sprintf("127.0.3.%v", i+10)), Port: port}, iter)
		require.Nil(t, err)
		require.NotNil(t, cs[i])
		defer cs[i].Close()

		_, err = cs[i].Advertise(ctx, ns, discovery.TTL(ttl))
		require.Nil(t, err)
	}

	finder, err := cs[0].FindPeers(ctx, ns)
	require.Nil(t, err)
	require.NotNil(t, finder)

	n := 0
	for range finder {
		n++
	}
	require.Equal(t, N, n)
}

func newTestHost() host.Host {
	h, err := libp2p.New(
		libp2p.NoListenAddrs,
		libp2p.NoTransports,
		libp2p.Transport(inproc.New()),
		libp2p.ListenAddrStrings("/inproc/~"))
	if err != nil {
		panic(err)
	}

	return h
}
