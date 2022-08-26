package pex

import (
	"errors"
	"fmt"

	"capnproto.org/go/capnp/v3"
	ds "github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/record"
	"github.com/wetware/casm/internal/api/pex"
)

type GossipRecord struct {
	g pex.Gossip
	peer.PeerRecord
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

	_, s, err := capnp.NewMessage(capnp.SingleSegment(make([]byte, 0, 512)))
	if err != nil {
		return nil, err
	}

	g, err := pex.NewRootGossip(s)
	if err != nil {
		return nil, err
	}

	b, err := env.Marshal()
	if err == nil {
		err = g.SetEnvelope(b)
	}

	return &GossipRecord{
		g:          g,
		PeerRecord: *rec,
	}, err
}

func (g *GossipRecord) Loggable() map[string]interface{} {
	return map[string]interface{}{
		"peer_id": g.PeerID,
		"hop":     g.Hop(),
		"seq":     g.Seq,
	}
}

func (g *GossipRecord) Key() ds.Key {
	return ds.NewKey(g.PeerID.String())
}

func (g *GossipRecord) Hop() uint64 { return g.g.Hop() }
func (g *GossipRecord) IncrHop()    { g.g.SetHop(g.g.Hop() + 1) }

// Distance returns the XOR of the last byte from 'id' and the record's ID.
func (g *GossipRecord) Distance(id peer.ID) uint64 {
	return lastUint64(string(g.PeerID)) ^ lastUint64(string(id))
}

func (g *GossipRecord) Message() *capnp.Message { return g.g.Message() }

func (g *GossipRecord) ReadMessage(m *capnp.Message) error {
	var err error
	if g.g, err = pex.ReadRootGossip(m); err != nil {
		return err
	}

	b, err := g.g.Envelope()
	if err != nil {
		return err
	}

	e, err := record.ConsumeTypedEnvelope(b, &g.PeerRecord)
	if err != nil {
		return err
	}

	// is record self-signed?
	if g.PeerID.MatchesPublicKey(e.PublicKey) {
		return nil
	}

	return &ValidationError{
		Cause: fmt.Errorf("%w: peer id does not match public key for record",
			record.ErrInvalidSignature),
	}
}

type View []*GossipRecord

func (v View) Len() int           { return len(v) }
func (v View) Less(i, j int) bool { return v[i].Hop() < v[j].Hop() }
func (v View) Swap(i, j int)      { v[i], v[j] = v[j], v[i] }

func (v View) PeerRecords() []*peer.PeerRecord {
	recs := make([]*peer.PeerRecord, 0, v.Len())
	for _, p := range v {
		recs = append(recs, &p.PeerRecord)
	}
	return recs
}

// Validate a View that was received during a gossip round.
func (v View) Validate() error {
	if len(v) == 0 {
		return ValidationError{Message: "empty view"}
	}

	// Non-senders should have a hop > 0
	for _, g := range v[:len(v)-1] {
		if g.Hop() == 0 {
			return ValidationError{
				Message: fmt.Sprintf("peer %s", g.PeerID.ShortString()),
				Cause:   fmt.Errorf("%w: expected hop > 0", ErrInvalidRange),
			}
		}
	}

	// Validate sender hop == 0
	if g := v.last(); g.Hop() != 0 {
		return ValidationError{
			Message: fmt.Sprintf("sender %s", g.PeerID.ShortString()),
			Cause:   fmt.Errorf("%w: nonzero hop for sender", ErrInvalidRange),
		}
	}

	return nil
}

func (v View) Bind(f func(View) View) View { return f(v) }

func (v View) find(id peer.ID) (have *GossipRecord, found bool) {
	for _, have = range v {
		if found = id == have.PeerID; found {
			break
		}
	}

	return
}

// n.b.:  panics if v is empty.
func (v View) last() *GossipRecord { return v[len(v)-1] }

func (v View) incrHops() {
	for _, g := range v {
		g.IncrHop()
	}
}

func (v View) diff(other View) (diff View) {
	for _, g := range v {
		if _, found := other.find(g.PeerID); !found {
			diff = append(diff, g)
		}
	}
	return
}

// convert last 8 bytes of a peer.ID into a unit64.
func lastUint64(s string) (u uint64) {
	for i := 0; i < 8; i++ {
		u = (u << 8) | uint64(s[len(s)-i-1])
	}

	return
}
