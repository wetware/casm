package pex

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/peer"
	logtest "github.com/lthibault/log/test"
	syncutil "github.com/lthibault/util/sync"
	"github.com/stretchr/testify/require"
	mock_libp2p "github.com/wetware/casm/internal/mock/libp2p"
	"go.uber.org/fx/fxtest"
)

var (
	errTest       = errors.New("test")
	matchCtx      = gomock.AssignableToTypeOf(reflect.TypeOf((*context.Context)(nil)).Elem())
	matchDuration = gomock.AssignableToTypeOf(reflect.TypeOf((*time.Duration)(nil)).Elem())
)

func TestDiscovery_nil_instance_should_nop(t *testing.T) {
	t.Parallel()

	require.NoError(t, (*discover)(nil).Track(context.Background(), ns, 10))
}

func TestDiscovery(t *testing.T) {
	t.Parallel()
	t.Helper()

	for _, tt := range []struct {
		name string
		fn   discoveryTestFunc
	}{
		{
			name: "should_start_and_stop_without_error",
			fn:   func(*testing.T, *logtest.MockLogger, *mock_libp2p.MockDiscovery, *discover) {},
		},
		{
			name: "track_should_abort_with_expired_context",
			fn:   trackShouldAbortWithExpiredContext,
		},
		{
			name: "track_should_log_backoff_on_error",
			fn:   trackShouldLogBackoffOnError,
		},
		{
			name: "track_should_start_advertise_loop",
			fn:   trackShouldStartAdvertiseLoop,
		},
		{
			name: "discover_should_call_FindPeers",
			fn:   discoverShouldCallFindPeers,
		},
	} {
		runDiscoveryTest(t, tt.name, tt.fn)
	}
}

func discoverShouldCallFindPeers(t *testing.T, l *logtest.MockLogger, d *mock_libp2p.MockDiscovery, disc *discover) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := make(<-chan peer.AddrInfo)

	d.EXPECT().
		FindPeers(gomock.AssignableToTypeOf(ctx), ns).
		Return(ch, nil)

	ps, err := disc.Discover(ctx, ns)
	require.NoError(t, err)
	require.Equal(t, ch, ps)
}

func trackShouldLogBackoffOnError(t *testing.T, l *logtest.MockLogger, d *mock_libp2p.MockDiscovery, disc *discover) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	namespace := l.EXPECT().
		WithField("ns", ns).
		Return(l).
		Times(1)

	advertise := d.EXPECT().
		Advertise(matchCtx, ns).
		Return(time.Duration(0), errTest).
		After(namespace).
		Times(1)

	l.EXPECT().
		WithError(errTest).
		Return(l).
		After(advertise).
		Times(1)

	l.EXPECT().
		WithField("backoff", matchDuration).
		Return(l).
		After(advertise).
		Times(1)

	warning := l.EXPECT().
		Warn("bootstrap failed").
		After(advertise).
		Times(1)

	var ok syncutil.Flag
	d.EXPECT().
		Advertise(matchCtx, ns).
		DoAndReturn(func(context.Context, string, ...discovery.Option) (time.Duration, error) {
			ok.Set()
			return 0, context.Canceled
		}).
		After(warning).
		Times(1)

	err := disc.Track(ctx, ns, time.Second)
	require.NoError(t, err)
	require.Eventually(t, ok.Bool,
		time.Second*2, // see global backoff instance in discovery.go
		time.Millisecond*100)
}

func trackShouldAbortWithExpiredContext(t *testing.T, l *logtest.MockLogger, d *mock_libp2p.MockDiscovery, disc *discover) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := disc.Track(ctx, ns, time.Second)
	require.ErrorIs(t, err, context.Canceled)
}

func trackShouldStartAdvertiseLoop(t *testing.T, l *logtest.MockLogger, d *mock_libp2p.MockDiscovery, disc *discover) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	l.EXPECT().
		WithField("ns", ns).
		Times(1)

	const (
		ttl   = time.Millisecond * 5
		times = 5
	)

	var ok syncutil.Flag
	succeed := d.EXPECT().
		Advertise(matchCtx, ns).
		Return(ttl, nil).
		Times(times)

	d.EXPECT().
		Advertise(matchCtx, ns).
		DoAndReturn(func(context.Context, string, ...discovery.Option) (time.Duration, error) {
			ok.Set()
			return 0, context.Canceled
		}).
		After(succeed).
		Times(1)

	err := disc.Track(ctx, ns, time.Second)
	require.NoError(t, err)

	require.Eventually(t, ok.Bool, ttl*100, ttl)
}

type discoveryTestFunc func(*testing.T, *logtest.MockLogger, *mock_libp2p.MockDiscovery, *discover)

func runDiscoveryTest(t *testing.T, name string, f discoveryTestFunc) {
	t.Run(name, func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		l := logtest.NewMockLogger(ctrl)
		d := mock_libp2p.NewMockDiscovery(ctrl)

		lx := fxtest.NewLifecycle(t)

		disc := newDiscover(discoverParam{
			Log:  l,
			Disc: d,
		}, lx)

		err := lx.Start(context.Background())
		require.NoError(t, err)

		f(t, l, d, disc)

		err = lx.Stop(context.Background())
		require.NoError(t, err)
	})
}
