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

type HostTracker struct {
	host host.Host

	envelope atomic.Value

	once sync.Once
	sub  event.Subscription

	mu        sync.Mutex
	callbacks []Callback
}

type Callback func()

func New(h host.Host, callback ...Callback) *HostTracker {
	return &HostTracker{host: h, callbacks: callback}
}

func (h *HostTracker) Ensure(ctx context.Context) (err error) {
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

func (h HostTracker) Close() error {
	return h.sub.Close()
}

func (h HostTracker) Record() *peer.PeerRecord {
	var rec peer.PeerRecord

	envelope := h.envelope.Load().(*record.Envelope)
	envelope.TypedRecord(&rec)
	return &rec
}

func (h HostTracker) AddCallback(c Callback) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.callbacks = append(h.callbacks, c)
}

func (h HostTracker) callCallbacks() {
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