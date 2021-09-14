package pex

import (
	"errors"
	"fmt"

	"capnproto.org/go/capnp/v3"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/record"
	"github.com/wetware/casm/internal/api/pex"
)

type GossipRecord struct {
	g pex.Gossip
	peer.PeerRecord
	*record.Envelope
}

func NewGossipRecord(env *record.Envelope) (*GossipRecord, error) {
	r, err := env.Record()
	if err != nil {
		return nil, err
	}

	rec, ok := r.(*peer.PeerRecord)
	if !ok {
		return nil, errors.New("not a peer record")
	}

	_, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		return nil, err
	}

	g, err := pex.NewRootGossip(seg)
	if err != nil {
		return nil, err
	}

	b, err := env.Marshal()
	if err != nil {
		return nil, err
	}

	err = g.SetEnvelope(b)

	return &GossipRecord{
		g:          g,
		PeerRecord: *rec,
		Envelope:   env,
	}, err
}

func (g *GossipRecord) Loggable() map[string]interface{} {
	return map[string]interface{}{
		"peer_id": g.PeerID,
		"hop":     g.Hop(),
		"seq":     g.Seq,
	}
}

func (g *GossipRecord) Hop() uint64 { return g.g.Hop() }
func (g *GossipRecord) IncrHop()    { g.g.SetHop(g.g.Hop() + 1) }

// Distance returns the XOR of the last byte from 'id' and the record's ID.
func (g *GossipRecord) Distance(id peer.ID) uint64 {
	return lastUint64(string(g.PeerID)) ^ lastUint64(string(id))
}

func (g *GossipRecord) Message() *capnp.Message { return g.g.Message() }

func (g *GossipRecord) ReadMessage(m *capnp.Message) (err error) {
	if g.g, err = pex.ReadRootGossip(m); err != nil {
		return
	}

	var b []byte
	if b, err = g.g.Envelope(); err != nil {
		return
	}

	if g.Envelope, err = record.ConsumeTypedEnvelope(b, &g.PeerRecord); err != nil {
		return
	}

	// is record self-signed?
	if g.PeerID.MatchesPublicKey(g.Envelope.PublicKey) {
		return
	}

	return ValidationError{
		Cause: fmt.Errorf("%w: peer id does not match public key for record",
			record.ErrInvalidSignature),
	}
}
