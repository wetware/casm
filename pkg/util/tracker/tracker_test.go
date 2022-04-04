package tracker_test

import (
	"context"
	"testing"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/record"
	inproc "github.com/lthibault/go-libp2p-inproc-transport"
	"github.com/stretchr/testify/require"
	"github.com/wetware/casm/pkg/util/tracker"
)

func TestEnsure(t *testing.T) {
	t.Parallel()

	h, err := newTestHost()
	require.NoError(t, err)

	addrs := make(chan *peer.PeerRecord, 0)
	callback := func(rec *peer.PeerRecord) {
		addrs <- rec
	}

	tr := tracker.New(h, callback)
	waitReady(h)
	// until Ensure is not called, no record is tracked
	require.Nil(t, tr.Record())

	err = tr.Ensure(context.Background())
	require.NoError(t, err)

	// right after calling Ensure a record should be available
	require.NotNil(t, tr.Record())
}

func TestClose(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h, err := newTestHost()
	require.NoError(t, err)

	tr := tracker.New(h)

	err = tr.Ensure(ctx)
	require.NoError(t, err)
	rec := tr.Record()
	require.NotNil(t, rec)

	tr.Close()
	// after closing the record must be same
	require.Equal(t, rec, tr.Record())
}

func TestCallback(t *testing.T) {
	t.Parallel()

	t.Run("init callback", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		h, err := newTestHost()
		require.NoError(t, err)

		records := make(chan *peer.PeerRecord, 0)
		callback := func(rec *peer.PeerRecord) {
			records <- rec
		}

		tr := tracker.New(h, callback)

		err = tr.Ensure(ctx)
		require.NoError(t, err)
		defer tr.Close()

		rec := <-records
		println(h, rec)
		require.Equal(t, h.ID(), rec.PeerID)
	})

	t.Run("added callback after init", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		h, err := newTestHost()
		require.NoError(t, err)

		records := make(chan *peer.PeerRecord, 0)
		callback := func(rec *peer.PeerRecord) {
			records <- rec
		}

		tr := tracker.New(h)
		tr.AddCallback(callback)

		err = tr.Ensure(ctx)
		require.NoError(t, err)

		rec := <-records
		require.Equal(t, h.ID(), rec.PeerID)
	})
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
