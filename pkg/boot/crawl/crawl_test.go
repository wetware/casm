package crawl_test

import (
	// "context"
	// "fmt"
	// "net"
	"testing"
	// "time"

	// "github.com/libp2p/go-libp2p"
	// "github.com/libp2p/go-libp2p-core/discovery"
	// "github.com/libp2p/go-libp2p-core/host"
	// inproc "github.com/lthibault/go-libp2p-inproc-transport"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
	"github.com/wetware/casm/pkg/boot/crawl"
)

func TestMultiaddr(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		addr string
		fail bool
	}{
		{"/ip4/228.8.8.8/udp/8822/cidr/32", false},
		{"/ip4/228.8.8.8/udp/8822/cidr/129", true},
	} {
		_, err := ma.NewMultiaddr(tt.addr)
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

		s, err := crawl.TranscoderCIDR{}.BytesToString([]byte{0x00, 0x00})
		assert.ErrorIs(t, err, crawl.ErrCIDROverflow,
			"should not parse byte arrays of length > 1")
		assert.Empty(t, s)

		s, err = crawl.TranscoderCIDR{}.BytesToString([]byte{0xFF})
		assert.ErrorIs(t, err, crawl.ErrCIDROverflow,
			"should not validate CIDR greater than 128")
		assert.Empty(t, s)

		s, err = crawl.TranscoderCIDR{}.BytesToString([]byte{0x01})
		assert.NoError(t, err, "should parse CIDR of 1")
		assert.Equal(t, "1", s, "should return \"1\"")
	})

	t.Run("BytesToString", func(t *testing.T) {
		t.Parallel()

		b, err := crawl.TranscoderCIDR{}.StringToBytes("fail")
		assert.Error(t, err,
			"should not validate non-numerical strings")
		assert.Nil(t, b)

		b, err = crawl.TranscoderCIDR{}.StringToBytes("255")
		assert.ErrorIs(t, err, crawl.ErrCIDROverflow,
			"should not validate string '255'")
		assert.Nil(t, b)
	})

	t.Run("ValidateBytes", func(t *testing.T) {
		t.Parallel()

		err := crawl.TranscoderCIDR{}.ValidateBytes([]byte{0x00})
		assert.NoError(t, err,
			"should validate CIDR block of 0")

		err = crawl.TranscoderCIDR{}.ValidateBytes([]byte{0xFF})
		assert.ErrorIs(t, err, crawl.ErrCIDROverflow,
			"should not validate CIDR blocks greater than 128")
	})
}

// func TestOne(t *testing.T) {
// 	t.Parallel()
// 	t.Helper()

// 	var (
// 		ip          = net.ParseIP("127.0.1.10")
// 		cidr string = fmt.Sprintf("%v/24", ip.String())
// 		port        = 8822
// 		addr        = &net.UDPAddr{IP: ip, Port: port}
// 		ns   string = "one"
// 	)

// 	ctx, cancel := context.WithCancel(context.Background())
// 	defer cancel()

// 	h := newTestHost()

// 	iter, err := crawl.NewCIDR(cidr, port)
// 	require.NoError(t, err)
// 	require.NotNil(t, iter)

// 	c, err := crawl.New(h, addr, iter)
// 	require.NoError(t, err)
// 	require.NotNil(t, c)
// 	defer c.Close()

// 	require.NotNil(t, c)
// 	finder, err := c.FindPeers(ctx, ns)
// 	require.NoError(t, err)
// 	require.NotNil(t, finder)

// 	n := 0
// 	for range finder {
// 		n++
// 	}
// 	require.Equal(t, 0, n)
// }

// func TestTwo(t *testing.T) {
// 	t.Parallel()
// 	t.Helper()

// 	var (
// 		ip0          = net.ParseIP("127.0.2.10")
// 		ip1          = net.ParseIP("127.0.2.11")
// 		port         = 8822
// 		addr0        = &net.UDPAddr{IP: ip0, Port: port}
// 		addr1        = &net.UDPAddr{IP: ip1, Port: port}
// 		ns    string = "two"
// 		ttl          = time.Hour
// 	)

// 	ctx, cancel := context.WithCancel(context.Background())
// 	defer cancel()

// 	h0 := newTestHost()
// 	h1 := newTestHost()

// 	iter0, err := crawl.NewCIDR("127.0.2.0/24", port)
// 	require.NoError(t, err)

// 	c0, err := crawl.New(h0, addr0, iter0)
// 	require.NoError(t, err)
// 	require.NotNil(t, c0)
// 	defer c0.Close()

// 	iter1, err := crawl.NewCIDR("127.0.2.0/24", port)
// 	require.NoError(t, err)

// 	c1, err := crawl.New(h1, addr1, iter1)
// 	require.NoError(t, err)
// 	require.NotNil(t, c0)
// 	defer c1.Close()

// 	_, err = c1.Advertise(ctx, ns, discovery.TTL(ttl))
// 	require.NoError(t, err)

// 	finder, err := c0.FindPeers(ctx, ns)
// 	require.NoError(t, err)
// 	require.NotNil(t, finder)

// 	n := 0
// 	for info := range finder {
// 		require.EqualValues(t, h1.ID(), info.ID)
// 		n++
// 	}
// 	require.Equal(t, 1, n)
// }

// func TestMultiple(t *testing.T) {
// 	t.Parallel()
// 	t.Helper()

// 	const (
// 		N           = 20
// 		port        = 8822
// 		ns   string = "multiple"
// 		ttl         = time.Hour
// 	)

// 	var (
// 		hs  = make([]host.Host, N)
// 		cs  = make([]*crawl.Crawler, N)
// 		err error
// 	)

// 	ctx, cancel := context.WithCancel(context.Background())
// 	defer cancel()

// 	for i := 0; i < N; i++ {
// 		hs[i] = newTestHost()
// 		defer hs[i].Close()

// 		iter, err := crawl.NewCIDR("127.0.3.0/24", port)
// 		require.NoError(t, err)

// 		cs[i], err = crawl.New(hs[i], &net.UDPAddr{IP: net.ParseIP(fmt.Sprintf("127.0.3.%v", i+10)), Port: port}, iter)
// 		require.NoError(t, err)
// 		require.NotNil(t, cs[i])
// 		defer cs[i].Close()

// 		_, err = cs[i].Advertise(ctx, ns, discovery.TTL(ttl))
// 		require.NoError(t, err)
// 	}

// 	finder, err := cs[0].FindPeers(ctx, ns)
// 	require.NoError(t, err)
// 	require.NotNil(t, finder)

// 	n := 0
// 	for range finder {
// 		n++
// 	}
// 	require.Equal(t, N, n)
// }

// func newTestHost() host.Host {
// 	h, err := libp2p.New(
// 		libp2p.NoListenAddrs,
// 		libp2p.NoTransports,
// 		libp2p.Transport(inproc.New()),
// 		libp2p.ListenAddrStrings("/inproc/~"))
// 	if err != nil {
// 		panic(err)
// 	}

// 	return h
// }
