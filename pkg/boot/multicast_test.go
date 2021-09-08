package boot_test

import (
	"context"
	"net"
	"testing"

	"capnproto.org/go/capnp/v3"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	mock_boot "github.com/wetware/casm/internal/mock/pkg/boot"
	"github.com/wetware/casm/pkg/boot"
	mx "github.com/wetware/matrix/pkg"
)

func TestNewTransport(t *testing.T) {
	t.Parallel()

	var c boot.Config
	c.Apply(nil)

	got, err := c.NewTransport()
	require.NoError(t, err)
	require.IsType(t, &boot.MulticastUDP{}, got)

	mudp := got.(*boot.MulticastUDP)
	require.Equal(t, net.IP{228, 8, 8, 8}, mudp.Group.IP.To4())
	require.Equal(t, 8822, mudp.Group.Port)
	require.Equal(t, "udp", mudp.Group.Network())
}

func TestMulticastClient_init(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	s := mock_boot.NewMockScatterer(ctrl)
	g := mock_boot.NewMockGatherer(ctrl)
	g.EXPECT().
		Gather(gomock.Any()).
		DoAndReturn(func(ctx context.Context) (*capnp.Message, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}).
		AnyTimes()

	tpt := mock_boot.NewMockTransport(ctrl)
	tpt.EXPECT().
		Dial().
		Return(s, nil).
		Times(1)
	tpt.EXPECT().
		Listen().
		Return(g, nil).
		Times(1)

	c, err := boot.NewMulticastClient(boot.WithTransport(tpt))
	require.NoError(t, err)
	require.NoError(t, c.Close())
}

func TestMulticast_init(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := mx.New(ctx).MustHost(ctx)

	s := mock_boot.NewMockScatterer(ctrl)
	g := mock_boot.NewMockGatherer(ctrl)
	g.EXPECT().
		Gather(gomock.Any()).
		DoAndReturn(func(ctx context.Context) (*capnp.Message, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}).
		AnyTimes()

	tpt := mock_boot.NewMockTransport(ctrl)
	tpt.EXPECT().
		Dial().
		Return(s, nil).
		Times(1)
	tpt.EXPECT().
		Listen().
		Return(g, nil).
		Times(1)

	m, err := boot.NewMulticast(h, boot.WithTransport(tpt))
	require.NoError(t, err)
	require.NoError(t, m.Close())
}
