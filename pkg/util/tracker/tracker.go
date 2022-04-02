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

const empty = 0

var ErrClosed = errors.New("closed")

type HostAddrTracker struct {
	host host.Host

	envelope atomic.Value

	sub event.Subscription

	mu        sync.Mutex
	callbacks []Callback

	closed bool
}

type Callback func(*peer.PeerRecord)

func New(h host.Host, callback ...Callback) *HostAddrTracker {
	return &HostAddrTracker{host: h, callbacks: callback}
}

func (h *HostAddrTracker) Ensure(ctx context.Context) (err error) {
	if h.closed { // only run Ensure logic once
		return ErrClosed
	}

	if h.sub != nil { // only run once the Ensure logic
		return
	}

	if len(h.host.Addrs()) == 0 {
		return errors.New("host not accepting connections")
	}

	h.sub, err = h.host.EventBus().Subscribe(new(event.EvtLocalAddressesUpdated))
	if err != nil {
		return err
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
			return fmt.Errorf("host %w", ErrClosed)
		}

		evt := v.(event.EvtLocalAddressesUpdated)
		h.envelope.Store(evt.SignedPeerRecord)
		h.callCallbacks()

	case <-ctx.Done():
		h.sub.Close()
		return ctx.Err()
	}

	go func() {
		for v := range h.sub.Out() {
			evt := v.(event.EvtLocalAddressesUpdated)
			h.envelope.Store(evt.SignedPeerRecord)
			h.callCallbacks()
		}
	}()

	h.closed = true

	return
}

func (h *HostAddrTracker) Close() (err error) {
	if h.sub != nil {
		err = h.sub.Close()
	}
	h.closed = false
	return
}

func (h *HostAddrTracker) Record() *peer.PeerRecord {
	rawEnvelope := h.envelope.Load()
	if rawEnvelope == nil {
		return nil
	}

	envelope := rawEnvelope.(*record.Envelope)

	var rec peer.PeerRecord
	envelope.TypedRecord(&rec)
	return &rec
}

func (h *HostAddrTracker) AddCallback(c Callback) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.callbacks = append(h.callbacks, c)
}

func (h *HostAddrTracker) callCallbacks() {
	h.mu.Lock()
	defer h.mu.Unlock()

	rec := h.Record()
	for _, c := range h.callbacks {
		go c(rec)
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
