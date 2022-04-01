package survey_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/record"
	inproc "github.com/lthibault/go-libp2p-inproc-transport"
	"github.com/stretchr/testify/require"
	mock_net "github.com/wetware/casm/internal/mock/net"
	"github.com/wetware/casm/pkg/boot/socket"
	"github.com/wetware/casm/pkg/boot/survey"
	"github.com/wetware/casm/pkg/util/tracker"
)

type test struct {
	h host.Host
	c *socket.RequestResponseCache
	t *tracker.HostAddrTracker
}

func TestClose(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	h, err := newTestHost()
	require.NoError(t, err)
	defer h.Close()

	conn := mock_net.NewMockPacketConn(ctrl)

	conn.EXPECT().Close().Times(1)
	conn.EXPECT().ReadFrom(gomock.Any()).AnyTimes()

	surveyor := survey.New(h, conn)
	surveyor.Close()
}

func TestAdvertise(t *testing.T) {
	t.Parallel()

	const (
		N  = 2
		ns = "test-advertise"
	)

	var (
		err  error
		addr = &net.UDPAddr{
			IP:   net.IPv4(127, 0, 0, 1),
			Port: 8822,
		}
	)

	tt := newTestTable(t, N)
	defer closeTestTable(tt)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn := mock_net.NewMockPacketConn(ctrl)

	request, err := loadRequest(tt[1], ns, 255)
	require.NoError(t, err)

	response, err := loadResponse(tt[0], ns)
	require.NoError(t, err)

	conn.EXPECT().ReadFrom(gomock.Any()).DoAndReturn(readIncomingRequest(request, addr)).MinTimes(1)
	conn.EXPECT().WriteTo(response, addr).MinTimes(1)
	conn.EXPECT().Close().Times(1)

	surveyor := survey.New(tt[0].h, conn)
	defer surveyor.Close()

	surveyor.Advertise(context.Background(), ns)

	time.Sleep(time.Second)
}

func TestFindPeers(t *testing.T) {
	t.Parallel()

	const (
		N  = 2
		ns = "test-findpeers"
	)

	var (
		err  error
		addr = &net.UDPAddr{
			IP:   net.IPv4(127, 0, 0, 1),
			Port: 8822,
		}
	)

	tt := newTestTable(t, N)
	defer closeTestTable(tt)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn := mock_net.NewMockPacketConn(ctrl)

	request, err := loadRequest(tt[0], ns, 255)
	require.NoError(t, err)

	response, err := loadResponse(tt[1], ns)
	require.NoError(t, err)

	conn.EXPECT().LocalAddr().Return(addr).Times(1)
	conn.EXPECT().WriteTo(request, addr).Times(1)
	conn.EXPECT().ReadFrom(gomock.Any()).DoAndReturn(readIncomingRequest(response, addr)).MinTimes(1)
	conn.EXPECT().Close().Times(1)

	surveyor := survey.New(tt[0].h, conn)
	defer surveyor.Close()

	peers, err := surveyor.FindPeers(context.Background(), ns, discovery.Limit(1))
	require.NoError(t, err)

	peersAmount := 0
	for info := range peers {
		require.EqualValues(t, tt[1].h.Addrs(), info.Addrs)
		require.EqualValues(t, tt[1].h.ID(), info.ID)

		peersAmount++
	}
	require.Equal(t, 1, peersAmount)
}

func newTestTable(t *testing.T, N int) []test {
	var (
		tt  = make([]test, N)
		err error
	)

	for i := 0; i < N; i++ {
		tt[i].h, err = newTestHost()
		require.NoError(t, err)

		_, err = waitReady(tt[i].h)
		require.NoError(t, err)

		tt[i].c, err = socket.NewCache(2)
		require.NoError(t, err)

		tt[i].t = tracker.New(tt[i].h)
		tt[i].t.Ensure(context.Background())
	}
	return tt
}

func closeTestTable(tt []test) {
	for _, t := range tt {
		t.h.Close()
	}
}

func newTestHost() (host.Host, error) {
	h, err := libp2p.New(
		libp2p.NoListenAddrs,
		libp2p.NoTransports,
		libp2p.Transport(inproc.New()),
		libp2p.ListenAddrStrings("/inproc/~"))
	if err != nil {
		return nil, err
	}

	return h, nil
}

func waitReady(h host.Host) (*record.Envelope, error) {
	sub, err := h.EventBus().Subscribe(new(event.EvtLocalAddressesUpdated))
	if err != nil {
		return nil, err
	}
	defer sub.Close()

	v := <-sub.Out()
	return v.(event.EvtLocalAddressesUpdated).SignedPeerRecord, nil
}

func readIncomingRequest(request []byte, from net.Addr) func([]byte) (int, net.Addr, error) {
	return func(b []byte) (n int, addr net.Addr, err error) {
		err = nil
		n = copy(b, request)
		addr = from
		return
	}
}

func loadRequest(t test, ns string, distance uint8) ([]byte, error) {
	request, err := t.c.LoadSurveyRequest(sealer(t.h), t.h.ID(), ns, distance)
	if err != nil {
		return nil, err
	}
	return request.Marshal()
}

func loadResponse(t test, ns string) ([]byte, error) {
	response, err := t.c.LoadResponse(sealer(t.h), t.t, ns)
	if err != nil {
		return nil, err
	}
	return response.Marshal()
}

func sealer(h host.Host) socket.Sealer {
	return func(r record.Record) (*record.Envelope, error) {
		return record.Seal(r, h.Peerstore().PrivKey(h.ID()))
	}
}
