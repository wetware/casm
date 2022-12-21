package casm_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"capnproto.org/go/capnp/v3"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	inproc "github.com/lthibault/go-libp2p-inproc-transport"
	"github.com/multiformats/go-multistream"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/golang/mock/gomock"
	mock_casm "github.com/wetware/casm/internal/mock/pkg"

	testing_api "github.com/wetware/casm/internal/api/testing"
	casm "github.com/wetware/casm/pkg"
)

func TestID(t *testing.T) {
	t.Parallel()
	t.Helper()

	var id = casm.ID(42)

	t.Run("Format", func(t *testing.T) {
		t.Parallel()

		assert.Len(t, id.Bytes(), 8, "should be 8 bytes long")
		assert.Len(t, id.String(), 16, "should be string of length 16")
	})

	t.Run("JSON", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		err := json.NewEncoder(&buf).Encode(id)
		require.NoError(t, err, "should marshal JSON")

		var got casm.ID
		err = json.NewDecoder(&buf).Decode(&got)
		require.NoError(t, err, "should unmarshal JSON")
		assert.Equal(t, id, got, "IDs should be equal")
	})
}

func TestVat(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	metrics := mock_casm.NewMockMetricReporter(ctrl)
	metrics.EXPECT().
		Incr("rpc./casm/0.0.0/casm/echo/packed.open").
		AnyTimes()
	metrics.EXPECT().
		Decr("rpc./casm/0.0.0/casm/echo/packed.open").
		AnyTimes()
	metrics.EXPECT().
		Incr("rpc.connect").
		AnyTimes()
	metrics.EXPECT().
		Incr("rpc.disconnect").
		AnyTimes()

	const ns = "" // test use of default namespace 'casm'

	// Set up server vat
	sv, err := casm.New(ns, server())
	require.NoError(t, err, "must create server vat")
	require.NotZero(t, sv, "server vat must be populated")
	defer sv.Host.Close()

	assert.Equal(t, "casm", sv.NS, "should use default namespace")

	sv.Metrics = metrics

	// Set up client vat
	cv, err := casm.New(ns, client())
	require.NoError(t, err, "must create client vat")
	require.NotZero(t, cv, "client vat must be populated")
	defer cv.Host.Close()

	assert.Equal(t, "casm", sv.NS, "should use default namespace")

	t.Run("Export", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sv.Export(echoer(), echoServer{})
		cv.Export(echoer(), echoServer{})

		conn, err := cv.Connect(ctx, addr(sv), echoer())
		require.NoError(t, err, "should connect to remote vat")
		defer conn.Close()

		e := testing_api.Echoer(conn.Bootstrap(ctx))
		defer e.Release()

		f, release := e.Echo(ctx, payload("hello, world!"))
		defer release()

		require.NoError(t, casm.Future(f).Err(), "echo call should succeed")
	})

	t.Run("Embargo", func(t *testing.T) {
		/*
			HACK:  Embargo is asynchronous; use assert.Eventually.
			TODO:  https://github.com/wetware/casm/issues/45
		*/
		sv.Embargo(echoer())
		assert.Eventually(t, func() bool {
			conn, err := cv.Connect(context.Background(), addr(sv), echoer())
			if err == nil || !errors.Is(err, multistream.ErrNotSupported) {
				conn.Close()
				return false
			}
			return true
		}, time.Second, time.Millisecond*100)
	})

	t.Run("InvalidNS", func(t *testing.T) {
		conn, err := cv.Connect(context.Background(), addr(sv), invalid())
		require.ErrorIs(t, err, casm.ErrInvalidNS)
		require.Nil(t, conn, "should not return a capability conn")
	})
}

func addr(v casm.Vat) peer.AddrInfo {
	return *host.InfoFromHost(v.Host)
}

func payload(s string) func(testing_api.Echoer_echo_Params) error {
	return func(ps testing_api.Echoer_echo_Params) error {
		return ps.SetPayload(s)
	}
}

func echoer() casm.BasicCap {
	return casm.BasicCap{
		// TODO:  /echo/lz4,
		"/echo/packed",
		"/echo",
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
