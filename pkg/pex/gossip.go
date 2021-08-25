package pex

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"sort"

	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/record"
)

const (
	viewRecordEnvelopeDomain   = "casm/pex/view"
	gossipRecordEnvelopeDomain = "casm/pex/gossip"
)

var (
	// TODO:  verify that these identifiers are available.
	// https://github.com/multiformats/multicodec/blob/master/table.csv
	viewRecordEnvelopePayloadType   = []byte{0x03, 0x04}
	gossipRecordEnvelopePayloadType = []byte{0x03, 0x03}

	// ErrInvalidRange is returned as a cause in a ValidationError when
	// a field's value falls outside the expected range.
	ErrInvalidRange = errors.New("invalid range")
)

func init() {
	record.RegisterType(&GossipRecord{})
	record.RegisterType(&View{})
}

type GossipRecord struct {
	Hop uint64
	peer.PeerRecord
	*record.Envelope
}

func NewGossipRecordFromEvent(ev event.EvtLocalAddressesUpdated) (GossipRecord, error) {
	g := GossipRecord{Envelope: ev.SignedPeerRecord}
	return g, g.Envelope.TypedRecord(&g.PeerRecord)
}

func (g *GossipRecord) Loggable() map[string]interface{} {
	return map[string]interface{}{
		"peer":  g.PeerRecord.PeerID,
		"addrs": g.PeerRecord.Addrs,
		"seq":   g.PeerRecord.Seq,
		"hop":   g.Hop,
	}
}

// Distance returns the XOR of the last byte from 'id' and the record's ID.
func (g *GossipRecord) Distance(id peer.ID) uint8 {
	return lastbyte(g.PeerID) ^ lastbyte(id)
}

// Domain is the "signature domain" used when signing and verifying a particular
// Record type. The Domain string should be unique to your Record type, and all
// instances of the Record type must have the same Domain string.
func (g *GossipRecord) Domain() string { return gossipRecordEnvelopeDomain }

// Codec is a binary identifier for this type of record, ideally a registered multicodec
// (see https://github.com/multiformats/multicodec).
// When a Record is put into an Envelope (see record.Seal), the Codec value will be used
// as the Envelope's PayloadType. When the Envelope is later unsealed, the PayloadType
// will be used to lookup the correct Record type to unmarshal the Envelope payload into.
func (g *GossipRecord) Codec() []byte { return gossipRecordEnvelopePayloadType }

// MarshalRecord converts a Record instance to a []byte, so that it can be used as an
// Envelope payload.
func (g *GossipRecord) MarshalRecord() ([]byte, error) {
	b := make([]byte, binary.MaxVarintLen64) // temp buffer for varints
	buf := bytes.Buffer{}                    // TODO(performance):  pool ?

	// encode fixed-length fields in big-endian binary
	n := binary.PutUvarint(b, g.Hop)
	buf.Write(b[:n])

	// marshal & encode as binary netstring
	tmp, err := g.Envelope.Marshal()
	if err != nil {
		return nil, err
	}

	n = binary.PutVarint(b, int64(len(tmp)))
	buf.Write(b[:n])
	buf.Write(tmp)

	return buf.Bytes(), nil
}

// UnmarshalRecord unmarshals a []byte payload into an instance of a particular Record type.
func (g *GossipRecord) UnmarshalRecord(b []byte) (err error) {
	var (
		n     int64
		r     = bytes.NewReader(b)
		maybe breaker
	)

	for _, fn := range []func(){
		func() { g.Hop, maybe.Err = binary.ReadUvarint(r) },
		func() { n, maybe.Err = binary.ReadVarint(r) },
		func() { b, maybe.Err = ioutil.ReadAll(io.LimitReader(r, n)) },
		func() { g.Envelope, maybe.Err = record.ConsumeTypedEnvelope(b, &g.PeerRecord) },
		func() { maybe.Err = validateIsSignedByPeer(g.PeerID, g.Envelope.PublicKey) },
	} {
		maybe.Do(fn)
	}

	return maybe.Err
}

func (g GossipRecord) newSelector(id peer.ID) func(View) View {
	// P = .5
	if g.Distance(id) > 128 {
		return func(v View) View {
			sort.Sort(v)
			return v
		}
	}

	return func(v View) View {
		rand.Shuffle(len(v), v.Swap)
		return v
	}
}

type View []GossipRecord

func (v View) Loggable() map[string]interface{} {
	return map[string]interface{}{
		"peers": v.IDs(),
	}
}

func (v View) Len() int           { return len(v) }
func (v View) Less(i, j int) bool { return v[i].Hop < v[j].Hop }
func (v View) Swap(i, j int)      { v[i], v[j] = v[j], v[i] }

// Validate a View that was received during a gossip round.
//
// Note that validation is expected to fail if 'e' is not signed
// by the sender, i.e. the last peer in the view.  For this reason
// the PeerExchange.View().Validate() always fails.
func (v View) Validate(e *record.Envelope) error {
	// Validate outer signature.  This closes an attack vector whereby
	// a network operator corrupts the hop field of legitimate peers so
	// that they will be blacklisted by others.
	if err := validateIsSignedByPeer(v.last().PeerID, e.PublicKey); err != nil {
		return ValidationError{
			Cause: fmt.Errorf("%w: %s", record.ErrInvalidSignature, err),
		}
	}

	// Validate sender hop == 0
	if v.last().Hop != 0 {
		return ValidationError{
			Message: fmt.Sprintf("sender %s", v.last().PeerID.ShortString()),
			Cause:   fmt.Errorf("%w: nonzero hop for sender", ErrInvalidRange),
		}
	}

	// Validate hops from other peers > 0
	for _, g := range v[:len(v)-1] {
		if g.Hop == 0 {
			return ValidationError{
				Message: fmt.Sprintf("peer %s", g.PeerID.ShortString()),
				Cause:   fmt.Errorf("%w: expected hop > 0", ErrInvalidRange),
			}
		}
	}

	return nil
}

// Domain is the "signature domain" used when signing and verifying a particular
// Record type. The Domain string should be unique to your Record type, and all
// instances of the Record type must have the same Domain string.
func (v View) Domain() string { return viewRecordEnvelopeDomain }

// Codec is a binary identifier for this type of record, ideally a registered multicodec
// (see https://github.com/multiformats/multicodec).
// When a Record is put into an Envelope (see record.Seal), the Codec value will be used
// as the Envelope's PayloadType. When the Envelope is later unsealed, the PayloadType
// will be used to lookup the correct Record type to unmarshal the Envelope payload into.
func (v View) Codec() []byte { return viewRecordEnvelopePayloadType }

// MarshalRecord converts a Record instance to a []byte, so that it can be used as an
// Envelope payload.
func (v View) MarshalRecord() (b []byte, err error) {
	var (
		body []byte
		hdr  = make([]byte, binary.MaxVarintLen64)
	)

	for _, g := range v {
		if body, err = g.MarshalRecord(); err != nil {
			break
		}

		n := binary.PutVarint(hdr, int64(len(body)))
		b = append(b, hdr[:n]...)
		b = append(b, body...)
	}

	return
}

// UnmarshalRecord unmarshals a []byte payload into an instance of a particular Record type.
func (v *View) UnmarshalRecord(b []byte) (err error) {
	var (
		r = bytes.NewReader(b)
		g GossipRecord
		n int64
	)

	for {
		if n, err = binary.ReadVarint(r); err != nil {
			break
		}

		if b, err = ioutil.ReadAll(io.LimitReader(r, n)); err != nil {
			break
		}

		if err = g.UnmarshalRecord(b); err != nil {
			break
		}

		*v = append(*v, g)
	}

	if errors.Is(err, io.EOF) {
		err = nil
	}

	return
}

func (v View) IDs() peer.IDSlice {
	ps := make(peer.IDSlice, len(v))
	for i, p := range v {
		ps[i] = p.PeerID
	}
	return ps
}

func (v View) find(g GossipRecord) (have GossipRecord, found bool) {
	seek := g.PeerID
	for _, have = range v {
		if found = seek == have.PeerID; found {
			break
		}
	}

	return
}

// n.b.:  panics if v is empty.
func (v View) last() GossipRecord { return v[len(v)-1] }

func (v View) incrHops() {
	for i := range v {
		v[i].Hop++
	}
}

func validateIsSignedByPeer(want peer.ID, pk crypto.PubKey) error {
	got, err := peer.IDFromPublicKey(pk)
	if err != nil {
		return fmt.Errorf("pubkey: %w", err)
	}

	if want != got {
		return fmt.Errorf("record not signed by %s", want.ShortString())
	}

	return nil
}
