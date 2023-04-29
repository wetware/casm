package socket_test

import (
	"errors"
	"net"
	"reflect"
	"testing"

	"github.com/libp2p/go-libp2p/core/record"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/golang/mock/gomock"
	mock_net "github.com/wetware/casm/internal/mock/net"
	mock_socket "github.com/wetware/casm/internal/mock/pkg/socket"

	"github.com/wetware/casm/pkg/socket"
)

var (
	recordType      = reflect.TypeOf((*record.Record)(nil)).Elem()
	matchRecordType = gomock.AssignableToTypeOf(recordType)

	byteType       = reflect.TypeOf((*[]byte)(nil)).Elem()
	matchBytesType = gomock.AssignableToTypeOf(byteType)

	addrType      = reflect.TypeOf((*net.Addr)(nil)).Elem()
	matchAddrType = gomock.AssignableToTypeOf(addrType)
)

func TestSocket(t *testing.T) {
	t.Parallel()
	t.Helper()

	t.Run("Close", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn := mock_net.NewMockPacketConn(ctrl)
		conn.EXPECT().
			Close().
			Return(nil).
			Times(1)

		err := socket.Socket{Conn: conn}.Close()
		assert.NoError(t, err, "should close gracefully")

		conn.EXPECT().
			Close().
			Return(errors.New("test")).
			Times(1)

		err = socket.Socket{Conn: conn}.Close()
		assert.EqualError(t, err, "test", "should report error")
	})

	t.Run("Read", func(t *testing.T) {
		t.Parallel()

		t.Skip("testy not implemented")

		// ctrl := gomock.NewController(t)
		// defer ctrl.Finish()

		// conn := mock_net.NewMockPacketConn(ctrl)
		// conn.EXPECT().
		// 	ReadFrom(matchRecordType).
		// 	DoAndReturn(func(b []byte) (int, net.Addr, error) {
		// 		// ...
		// 	}).
		// 	Times(1)
	})

	t.Run("Write", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		addr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 9001}

		sealer := mock_socket.NewMockSealer(ctrl)
		sealer.EXPECT().
			Seal(matchRecordType).
			Return([]byte("test payload"), nil).
			Times(1)

		conn := mock_net.NewMockPacketConn(ctrl)
		conn.EXPECT().
			WriteTo(matchBytesType, matchAddrType).
			DoAndReturn(func(b []byte, a net.Addr) (int, net.Addr, error) {
				assert.Equal(t, addr, a)
				assert.Equal(t, "test payload", string(b))
				return len(b), addr, nil
			}).
			Times(1)

		r := mockRecord("test")
		err := socket.Socket{
			Conn:   conn,
			Sealer: sealer,
		}.Write(&r, addr)
		require.NoError(t, err)
	})
}
