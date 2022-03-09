package survey_test

import (
	"context"
	"net"
	"testing"
	"time"

	"capnproto.org/go/capnp/v3"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/record"
	inproc "github.com/lthibault/go-libp2p-inproc-transport"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	mock_net "github.com/wetware/casm/internal/mock/net"

	api "github.com/wetware/casm/internal/api/survey"
	"github.com/wetware/casm/pkg/boot/survey"
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

func TestSurveyor(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	h := newTestHost()
	defer h.Close()

	sub, err := h.EventBus().Subscribe(new(event.EvtLocalAddressesUpdated))
	require.NoError(t, err, "must subscribe to address updates")
	defer sub.Close()
	e := (<-sub.Out()).(event.EvtLocalAddressesUpdated).SignedPeerRecord

	mockTransport := survey.Transport{
		DialFunc: func(net.Addr) (net.PacketConn, error) {
			conn := mock_net.NewMockPacketConn(ctrl)
			// Expect a single call to Close
			conn.EXPECT().
				Close().
				Return(error(nil)).
				Times(1)

			// Expect a single REQUEST packet to be issued.
			conn.EXPECT().
				WriteTo(gomock.AssignableToTypeOf([]byte{}), gomock.AssignableToTypeOf(net.Addr(new(net.UDPAddr)))).
				Return(0, nil). // n not checked by sender
				Times(1)

			return conn, nil
		},
		ListenFunc: func(net.Addr) (net.PacketConn, error) {
			conn := mock_net.NewMockPacketConn(ctrl)
			// Expect a single call to Close
			conn.EXPECT().
				Close().
				Return(error(nil)).
				Times(1)

			// Expect one RESPONSE message.
			read := conn.EXPECT().
				ReadFrom(gomock.AssignableToTypeOf([]byte{})).
				DoAndReturn(func(b []byte) (int, net.Addr, error) {
					return copy(b, newResponsePayload(e)), new(net.UDPAddr), nil
				}).
				Times(1)

			conn.EXPECT().
				ReadFrom(gomock.AssignableToTypeOf([]byte{})).
				After(read).
				DoAndReturn(func(b []byte) (int, net.Addr, error) {
					<-ctx.Done()
					return 0, nil, survey.ErrClosed
				}).
				AnyTimes()

			return conn, nil
		},
	}

	s, err := survey.New(h, new(net.UDPAddr), survey.WithTransport(mockTransport))
	require.NoError(t, err, "should open packet connections")
	require.NotNil(t, s, "should return surveyor")
	defer s.Close()

	t.Run("ShouldAdvertise", func(t *testing.T) {
		ttl, err := s.Advertise(ctx, testNs, discovery.TTL(advertiseTTL))
		require.NoError(t, err, "should advertise successfully")
		require.Equal(t, advertiseTTL, ttl, "should return advertised TTL")
	})

	t.Run("ShouldFindPeer", func(t *testing.T) {
		finder, err := s.FindPeers(ctx, testNs)
		require.NoError(t, err, "should issue request packet")

		select {
		case <-time.After(time.Second * 10):
			t.Error("should receive response")
		case info := <-finder:
			// NOTE: we advertised h's record to avoid creating a separate host
			require.Equal(t, info, *host.InfoFromHost(h))
		}
	})

	t.Run("Close", func(t *testing.T) {
		require.NoError(t, s.Close(), "should close without error")

		ttl, err := s.Advertise(ctx, testNs)
		require.ErrorIs(t, err, survey.ErrClosed)
		require.Zero(t, ttl)

		ch, err := s.FindPeers(ctx, testNs)
		require.ErrorIs(t, err, survey.ErrClosed)
		require.Nil(t, ch)
	})
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

func newResponsePayload(e *record.Envelope) []byte {
	b, err := e.Marshal()
	if err != nil {
		panic(err)
	}

	m, s, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		panic(err)
	}

	p, err := api.NewRootPacket(s)
	if err != nil {
		panic(err)
	}

	if err = p.SetNamespace(testNs); err != nil {
		panic(err)
	}

	if err = p.SetResponse(b); err != nil {
		panic(err)
	}

	if b, err = m.MarshalPacked(); err != nil {
		panic(err)
	}

	return b
}
