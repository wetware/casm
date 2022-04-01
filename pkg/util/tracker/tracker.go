package tracker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/record"
)

var ErrClosed = errors.New("closed")

type HostAddrTracker struct {
	host host.Host

	envelope atomic.Value

	once sync.Once
	sub  event.Subscription

	mu        sync.Mutex
	callbacks []Callback
}

type Callback func()

func New(h host.Host, callback ...Callback) *HostAddrTracker {
	return &HostAddrTracker{host: h, callbacks: callback}
}

func (h *HostAddrTracker) Ensure(ctx context.Context) (err error) {
	h.once.Do(func() {
		if len(h.host.Addrs()) == 0 {
			err = errors.New("host not accepting connections")
			return
		}

		h.sub, err = h.host.EventBus().Subscribe(new(event.EvtLocalAddressesUpdated))
		if err != nil {
			return
		}

		// Ensure a sync operation is run before continuing the call
		// to Advertise, as this may otherwise cause a panic.
		//
		// The host may have unregistered its addresses concurrently
		// with the call to Subscribe, so we provide a cancellation
		// mechanism via the context.  Note that if the first call to
		// ensureTrackHostAddr fails, subsequent calls will also fail.
		select {
		case v, ok := <-h.sub.Out():
			if !ok {
				err = fmt.Errorf("host %w", ErrClosed)
				return
			}

			evt := v.(event.EvtLocalAddressesUpdated)
			h.envelope.Store(evt.SignedPeerRecord)
			h.callCallbacks()

		case <-ctx.Done():
			h.sub.Close()
			err = ctx.Err()
			return
		}

		go func() {
			for v := range h.sub.Out() {
				evt := v.(event.EvtLocalAddressesUpdated)
				h.envelope.Store(evt.SignedPeerRecord)
				h.callCallbacks()
			}
		}()

	})

	return
}

func (h HostAddrTracker) Close() error {
	if h.sub != nil {
		return h.sub.Close()
	}
	return nil
}

func (h HostAddrTracker) Record() *peer.PeerRecord {
	var rec peer.PeerRecord

	envelope := h.envelope.Load().(*record.Envelope)
	envelope.TypedRecord(&rec)
	return &rec
}

func (h HostAddrTracker) AddCallback(c Callback) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.callbacks = append(h.callbacks, c)
}

func (h HostAddrTracker) callCallbacks() {
	h.mu.Lock()
	defer h.mu.Unlock()

	for _, c := range h.callbacks {
		go c()
	}
}

// interface for retrieving the peer.PeerRecord
// from the Tracker and also a struct for testing purposes

type RecordProvider interface {
	Record() *peer.PeerRecord
}

type StaticRecordProvider struct {
	Envelope *record.Envelope
}

func (s StaticRecordProvider) Record() *peer.PeerRecord {
	var rec peer.PeerRecord

	s.Envelope.TypedRecord(&rec)
	return &rec
}
