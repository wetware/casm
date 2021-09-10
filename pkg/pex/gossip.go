package pex

import (
	"errors"
	"fmt"
	"math/rand"
	"sort"

	"capnproto.org/go/capnp/v3"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	ps "github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/libp2p/go-libp2p-core/record"
	"github.com/wetware/casm/internal/api/pex"
)

// DistanceProvider can report a distance metric between the local host
// and a remote peer.
type DistanceProvider interface {
	Distance(peer.ID) uint64
}

// ViewSelectorFactory is a factory type that dynamically creates a
// selector.  Implementations MUST ensure the returned selector always
// returns a view 'v' where v <= maxSize.
type ViewSelectorFactory func(h host.Host, d DistanceProvider, maxSize int) ViewSelector

func (f ViewSelectorFactory) Bind(next ViewSelector) ViewSelectorFactory {
	return func(h host.Host, d DistanceProvider, maxSize int) ViewSelector {
		return f(h, d, maxSize).Then(next)
	}
}

// ViewSelector implements the view selection strategy during a gossip round.
type ViewSelector func(View) View

// Select receives merger of the local view and the remote view as input,
// and returns the new view.
func (f ViewSelector) Select(v View) View { return f(v) }

// Then is a simple interface for composing selectors.
func (f ViewSelector) Then(next ViewSelector) ViewSelector {
	return func(v View) View {
		return next(f(v))
	}
}

// RandSelector uniformly shuffles the merged view.  If r == nil, the
// default global source is used.
//
// RandSelector is extremely robust against partitions, but slower to
// converge than 'TailSelector'.
func RandSelector(r *rand.Rand) ViewSelector {
	shuffle := rand.Shuffle
	if r != nil {
		shuffle = r.Shuffle
	}

	return func(v View) View {
		shuffle(len(v), v.Swap)
		return v
	}
}

// SortSelector returns the view sorted in descending order of hop.
// Elements towards the tail are more recent, and less diffused
// throughout the overlay.
func SortSelector() ViewSelector {
	return func(v View) View {
		sort.Sort(v)
		return v
	}
}

// TailSelector returns n elements from the view, starting at the
// tail.  Panics if n <= 0.
//
// When applied to a view that was previously sorted by SortSelector,
// the result is a selection strategy that converges quickly, but causes
// partitions to 'forget' about each other rapidly as well.
func TailSelector(n int) ViewSelector {
	if n <= 0 {
		panic("n must be greater than zero.")
	}

	return func(v View) View {
		if n < len(v) {
			v = v[len(v)-n:]
		}

		return v
	}
}

type GossipRecord struct {
	g pex.Gossip
	peer.PeerRecord
	*record.Envelope
}

func NewGossipRecord(h host.Host) (*GossipRecord, error) {
	cb, ok := ps.GetCertifiedAddrBook(h.Peerstore())
	if !ok {
		return nil, errNoSignedAddrs
	}

	env := cb.GetPeerRecord(h.ID())
	if env == nil {
		return nil, errors.New("record not found")
	}

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

func (g *GossipRecord) Hop() uint64 { return g.g.Hop() }
func (g *GossipRecord) IncrHop()    { g.g.SetHop(g.g.Hop() + 1) }

// Distance returns the XOR of the last byte from 'id' and the record's ID.
func (g *GossipRecord) Distance(id peer.ID) uint64 {
	return lastUint64(g.PeerID) ^ lastUint64(id)
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

type View []*GossipRecord

func (v View) Loggable() map[string]interface{} {
	return map[string]interface{}{
		"peers": v.IDs(),
	}
}

func (v View) Len() int           { return len(v) }
func (v View) Less(i, j int) bool { return v[i].Hop() < v[j].Hop() }
func (v View) Swap(i, j int)      { v[i], v[j] = v[j], v[i] }

// Validate a View that was received during a gossip round.
func (v View) Validate() error {
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

func (v View) IDs() peer.IDSlice {
	ps := make(peer.IDSlice, len(v))
	for i, p := range v {
		ps[i] = p.PeerID
	}
	return ps
}

func (v View) find(g *GossipRecord) (have *GossipRecord, found bool) {
	seek := g.PeerID
	for _, have = range v {
		if found = seek == have.PeerID; found {
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

// convert last 8 bytes of a peer.ID into a unit64.
func lastUint64(id peer.ID) (u uint64) {
	for i := 0; i < 8; i++ {
		u = (u << 8) | uint64(id[len(id)-i-1])
	}

	return
}
