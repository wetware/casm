package casm_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"capnproto.org/go/capnp/v3"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	inproc "github.com/lthibault/go-libp2p-inproc-transport"
	"github.com/multiformats/go-multistream"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/golang/mock/gomock"
	mock_casm "github.com/wetware/casm/internal/mock/pkg"

	testing_api "github.com/wetware/casm/internal/api/testing"
	casm "github.com/wetware/casm/pkg"
)

func TestVat(t *testing.T) {
	for _, tt := range []struct {
		name string
		cap  casm.BasicCap
	}{
		{
			name: "raw",
			cap:  casm.BasicCap{"/echo"},
		},
		{
			name: "packed",
			cap:  casm.BasicCap{"/echo/packed"},
		},
		{
			name: "lz4",
			cap:  casm.BasicCap{"/echo/lz4"},
		},
		{
			name: "packed and lz4",
			cap:  casm.BasicCap{"/echo/lz4"},
		},
	} {
		t.Run("TestVat"+tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			metrics := mock_casm.NewMockMetricReporter(ctrl)
			metrics.EXPECT().
				CountAdd(gomock.Any(), 1).
				AnyTimes()
			metrics.EXPECT().
				CountAdd(gomock.Any(), -1).
				AnyTimes()
			metrics.EXPECT().
				GaugeAdd(gomock.Any(), 1).
				AnyTimes()
			metrics.EXPECT().
				GaugeAdd(gomock.Any(), -1).
				AnyTimes()

			const ns = "" // test use of default namespace 'casm'

			// Set up server vat
			sv, err := casm.New(ns, server())
			require.NoError(t, err, "must create server vat")
			require.NotZero(t, sv, "server vat must be populated")
			assert.Equal(t, "casm", sv.Loggable()["ns"],
				"should use default namespace")

			sv.Metrics = metrics

			// Set up client vat
			cv, err := casm.New(ns, client())
			require.NoError(t, err, "must create client vat")
			require.NotZero(t, cv, "client vat must be populated")
			assert.Equal(t, "casm", sv.Loggable()["ns"],
				"should use default namespace")

			t.Run("Export", func(t *testing.T) {
				sv.Export(tt.cap, echoServer{})
				cv.Export(tt.cap, echoServer{})

				conn, err := cv.Connect(ctx, addr(sv), tt.cap)
				require.NoError(t, err, "should connect to remote vat")
				defer conn.Close()

				e := testing_api.Echoer(conn.Bootstrap(ctx))
				f, release := e.Echo(ctx, payload("hello, world!"))
				defer release()

				err = casm.Future{Future: f.Future}.Err()
				require.NoError(t, err, "echo call should succeed")
			})

			t.Run("Embargo"+tt.name, func(t *testing.T) {
				sv.Embargo(tt.cap)

				/*
					HACK:  Embargo is asynchronous.
					TODO:  https://github.com/wetware/casm/issues/45
				*/
				assert.Eventually(t, func() bool {
					conn, err := cv.Connect(ctx, addr(sv), tt.cap)
					return conn == nil && errors.Is(err, multistream.ErrNotSupported)
				}, time.Second, time.Millisecond*100)
			})

			t.Run("InvalidNS"+tt.name, func(t *testing.T) {
				conn, err := cv.Connect(ctx, addr(sv), invalid())
				require.ErrorIs(t, err, casm.ErrInvalidNS)
				require.Nil(t, conn, "should not return a capability conn")
			})
		})
	}
}

func addr(v casm.Vat) peer.AddrInfo {
	return *host.InfoFromHost(v.Host)
}

func payload(s string) func(testing_api.Echoer_echo_Params) error {
	return func(ps testing_api.Echoer_echo_Params) error {
		return ps.SetPayload(s)
	}
}

func invalid() casm.BasicCap {
	return casm.BasicCap{""}
}

type echoServer struct{}

func (echoServer) Client() capnp.Client {
	return capnp.Client(testing_api.Echoer_ServerToClient(echoServer{}))
}

func (echoServer) Echo(ctx context.Context, call testing_api.Echoer_echo) error {
	payload, err := call.Args().Payload()
	if err != nil {
		return err
	}

	res, err := call.AllocResults()
	if err != nil {
		return err
	}

	return res.SetResult(payload)
}

func client() casm.HostFactory {
	return casm.Client(
		libp2p.NoTransports,
		libp2p.Transport(inproc.New()))
}

func server() casm.HostFactory {
	return casm.Server(
		libp2p.NoTransports,
		libp2p.NoListenAddrs,
		libp2p.Transport(inproc.New()),
		libp2p.ListenAddrStrings("/inproc/~"))
}
