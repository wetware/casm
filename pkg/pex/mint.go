package pex

import (
	"context"
	"sync/atomic"

	"github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/wetware/casm/internal/api/pex"
)

// Mint sets the state ("mints") pex.Gossip instances based
// on the local peer's current PeerRecord.
type Mint interface {
	log.Loggable
	Load() *LocalPeer
	Mint(pex.Gossip) (*peer.PeerRecord, error)
}

type Mintable interface {
	SetEnvelope([]byte) error
}

type recordMint atomic.Value

func newRecordMint(ctx context.Context, h host.Host) (*recordMint, error) {
	// Host not serving connections?  This would cause <-sub.Out() to block.
	if len(h.Addrs()) == 0 {
		return nil, ErrNoListenAddrs
	}

	// Start minting gossip records
	sub, err := h.EventBus().Subscribe(new(event.EvtLocalAddressesUpdated))
	if err != nil {
		return nil, err
	}

	var m recordMint
	update := func(ctx context.Context) {
		select {
		case v := <-sub.Out():
			m.Update(v.(event.EvtLocalAddressesUpdated))

		case <-ctx.Done():
		}
	}

	// block until recordMint is initialized
	if update(ctx); ctx.Err() != nil {
		defer sub.Close()
		return nil, ctx.Err()
	}

	// update recordMint continuously in the background
	go func() {
		defer sub.Close()
		for ctx.Err() == nil {
			update(ctx)
		}
	}()

	return &m, nil
}

func (m *recordMint) Loggable() map[string]interface{} {
	return m.Load().Loggable()
}

func (m *recordMint) Update(ev event.EvtLocalAddressesUpdated) {
	b, _ := ev.SignedPeerRecord.Marshal()
	rec, _ := ev.SignedPeerRecord.Record()
	(*atomic.Value)(m).Store(&LocalPeer{
		RawEnvelope: b,
		Record:      rec.(*peer.PeerRecord),
	})
}

// Mint a pex.Gossip record, populating its 'Envelope' field, and
// returning the corresponding PeerRecord.
func (m *recordMint) Mint(rec Mintable) (*peer.PeerRecord, error) {
	p := m.Load()
	return p.Record, rec.SetEnvelope(p.RawEnvelope)
}

func (m *recordMint) Load() *LocalPeer {
	return (*atomic.Value)(m).Load().(*LocalPeer)
}

type LocalPeer struct {
	RawEnvelope []byte
	Record      *peer.PeerRecord
}

func (p LocalPeer) Loggable() map[string]interface{} {
	return map[string]interface{}{
		"id":  p.Record.PeerID,
		"seq": p.Record.Seq,
	}
}

// // minter populates GossipRecords with the current state of the
// // local PeerRecord
// type minter struct {
// 	env atomicEnvelope
// 	io.Closer
// }

// func newMinter(bus event.Bus) (*minter, error) {
// 	sub, err := bus.Subscribe(new(event.EvtLocalAddressesUpdated))
// 	if err != nil {
// 		return nil, err
// 	}

// 	v, ok := <-sub.Out() // stateful subscription; non-blocking
// 	if !ok {
// 		return nil, errors.New("closed")
// 	}

// 	var n = minter{Closer: sub}
// 	n.env.Consume(v.(event.EvtLocalAddressesUpdated))

// 	go func() {
// 		for v := range sub.Out() {
// 			n.env.Consume(v.(event.EvtLocalAddressesUpdated))
// 		}
// 	}()

// 	return &n, nil
// }

// func (n notary) SignedEnvelope(ctx context.Context) *record.Envelope {
// 	select {
// 		case
// 	}
// }
