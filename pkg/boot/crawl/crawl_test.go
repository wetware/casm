package crawl_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"reflect"
	"sync"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/libp2p/go-libp2p-core/record"
	inproc "github.com/lthibault/go-libp2p-inproc-transport"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	mock_net "github.com/wetware/casm/internal/mock/net"
	"github.com/wetware/casm/pkg/boot/crawl"
	"github.com/wetware/casm/pkg/boot/socket"
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

func TestCrawl_request_noadvert(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	sync := make(chan struct{})
	addr := &net.UDPAddr{
		IP:   net.IPv4(127, 0, 0, 1),
		Port: 8822,
	}

	h := newTestHost()
	defer h.Close()

	conn := mock_net.NewMockPacketConn(ctrl)
	conn.EXPECT().
		Close().
		DoAndReturn(bindClose(sync)).
		Times(1)

	readReq := conn.EXPECT().
		ReadFrom(gomock.Any()).
		DoAndReturn(bindIncomingRequest(addr)).
		Times(1)

	conn.EXPECT().
		ReadFrom(gomock.Any()).
		After(readReq).
		DoAndReturn(blockUntilClosed(sync)).
		AnyTimes()

	c := crawl.New(h, conn, crawl.WithStrategy(rangeUDP()))

	err := c.Close()
	assert.NoError(t, err, "should close gracefully")
}

func TestCrawl_request_advertised(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		syncClose  = make(chan struct{})
		syncAdvert = make(chan struct{})
		syncReply  = make(chan struct{})
	)

	addr := &net.UDPAddr{
		IP:   net.IPv4(127, 0, 0, 1),
		Port: 8822,
	}

	h := newTestHost()
	defer h.Close()

	conn := mock_net.NewMockPacketConn(ctrl)
	conn.EXPECT().
		Close().
		DoAndReturn(bindClose(syncClose)).
		Times(1)

	readReq := conn.EXPECT().
		ReadFrom(gomock.Any()).
		DoAndReturn(readIncomingRequestAfter(addr, syncAdvert)).
		Times(1)

	conn.EXPECT().
		WriteTo(matchOutgoingResponse(), gomock.Eq(addr)).
		After(readReq).
		DoAndReturn(sendError(nil, syncReply)).
		Times(1)

	conn.EXPECT().
		ReadFrom(gomock.Any()).
		After(readReq).
		Return(0, nil, net.ErrClosed).
		DoAndReturn(blockUntilClosed(syncClose)).
		AnyTimes()

	as := rangeUDP()
	c := crawl.New(h, conn, crawl.WithStrategy(as))

	ttl, err := c.Advertise(ctx, "casm")
	require.NoError(t, err, "advertise should succeed")
	assert.Equal(t, peerstore.TempAddrTTL, ttl)
	close(syncAdvert)

	<-syncReply

	err = c.Close()
	assert.NoError(t, err, "should close gracefully")
}

func bindClose(sync chan<- struct{}) func() error {
	return func() error {
		defer close(sync)
		return nil
	}
}

func blockUntilClosed(sync <-chan struct{}) func([]byte) (int, net.Addr, error) {
	return func([]byte) (int, net.Addr, error) {
		<-sync
		return 0, nil, net.ErrClosed
	}
}

var (
	reqBytes, gradualBytes, resBytes []byte
	initReq, initGradual, initRes    sync.Once
)

func bindIncomingRequest(from net.Addr) func([]byte) (int, net.Addr, error) {
	return func(b []byte) (n int, addr net.Addr, err error) {
		err = bindTestData(&initReq, &reqBytes, "../socket/testdata/request.golden.capnp")
		n = copy(b, reqBytes)
		addr = from
		return
	}
}

func readIncomingRequestAfter(from net.Addr, sync <-chan struct{}) func([]byte) (int, net.Addr, error) {
	bind := bindIncomingRequest(from)
	return func(b []byte) (int, net.Addr, error) {
		<-sync
		return bind(b)
	}
}

func matchOutgoingResponse() gomock.Matcher {
	return &matchResponse{}
}

type matchResponse struct {
	err error
}

// Matches returns whether x is a match.
func (m *matchResponse) Matches(x interface{}) bool {
	b, ok := x.([]byte)
	if !ok {
		m.err = fmt.Errorf("expected *boot.Record, got %s", reflect.TypeOf(x))
		return false
	}

	_, rec, err := record.ConsumeEnvelope(b, socket.EnvelopeDomain)
	if err != nil {
		m.err = fmt.Errorf("consume envelope: %w", err)
		return false
	}

	if _, ok = rec.(*socket.Record); !ok {
		m.err = fmt.Errorf("expected *boot.Record, got %s", reflect.TypeOf(rec))
		return false
	}

	return true
}

// String describes what the matcher matches.
func (m matchResponse) String() string {
	if m.err != nil {
		return m.err.Error()
	}

	return "is response packet"
}

func sendError(err error, sync chan<- struct{}) func([]byte, net.Addr) (int, error) {
	return func(b []byte, a net.Addr) (int, error) {
		defer close(sync)

		if err == nil {
			return len(b), nil
		}

		return 0, err
	}
}

func bindTestData(init *sync.Once, b *[]byte, path string) (err error) {
	init.Do(func() {
		if *b, err = ioutil.ReadFile(path); err != nil {
			panic(err)
		}
	})

	return
}

type mockRange struct {
	pos int
	as  []*net.UDPAddr
}

func rangeUDP(as ...*net.UDPAddr) crawl.Strategy {
	return func() (crawl.Range, error) {
		return &mockRange{as: as}, nil
	}
}

func (r *mockRange) Next(a net.Addr) bool {
	if r.pos == len(r.as) {
		return false
	}

	switch addr := a.(type) {
	case *net.UDPAddr:
		addr.IP = r.as[r.pos].IP
		addr.Zone = r.as[r.pos].Zone
		addr.Port = r.as[r.pos].Port
		return true
	}

	panic("unreachable")
}

func newTestHost() host.Host {
	h, err := libp2p.New(
		libp2p.NoListenAddrs,
		libp2p.NoTransports,
		libp2p.Transport(inproc.New()),
		libp2p.ListenAddrStrings("/inproc/test"))
	if err != nil {
		panic(err)
	}

	return h
}
