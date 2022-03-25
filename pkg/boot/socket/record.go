package socket

import (
	"capnproto.org/go/capnp/v3"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/wetware/casm/internal/api/boot"
)

const (
	EnvelopeDomain = "casm-boot-record"

	TypeRequest        RecordType = boot.Packet_Which_request
	TypeGradualRequest RecordType = boot.Packet_Which_gradualRequest
	TypeResponse       RecordType = boot.Packet_Which_response
)

type RecordType = boot.Packet_Which

// TODO:  register this once stable.
// https://github.com/multiformats/multicodec/blob/master/table.csv
var EnvelopePayloadType = []byte{0x1f, 0x00}

type Record boot.Packet

// func NewRequest(ns string, id peer.ID) (Packet, error) {
// 	p, err := newPacket(capnp.SingleSegment(nil), ns)
// 	if err != nil {
// 		return Packet{}, err
// 	}

// 	return Packet(p), p.SetRequest(string(id))
// }

// func NewGradualRequest(ns string, id peer.ID, dist uint8) (Packet, error) {
// 	p, err := newPacket(capnp.SingleSegment(nil), ns)
// 	if err != nil {
// 		return Packet{}, err
// 	}

// 	p.SetGradualRequest()
// 	p.GradualRequest().SetDistance(dist)
// 	return Packet(p), p.GradualRequest().SetFrom(string(id))
// }

func (r Record) Type() RecordType {
	return r.asPacket().Which()
}

func (r Record) asPacket() boot.Packet { return boot.Packet(r) }

func (r Record) Namespace() (string, error) {
	return boot.Packet(r).Namespace()
}

func (r Record) PeerID() (id peer.ID, err error) {
	switch r.asPacket().Which() {
	case boot.Packet_Which_response:
		return Response{r}.Peer()

	default:
		return Request{r}.From()
	}
}

// Domain is the "signature domain" used when signing and verifying a particular
// Record type. The Domain string should be unique to your Record type, and all
// instances of the Record type must have the same Domain string.
func (r Record) Domain() string { return EnvelopeDomain }

// Codec is a binary identifier for this type of record, ideally a registered multicodec
// (see https://github.com/multiformats/multicodec).
// When a Record is put into an Envelope (see record.Seal), the Codec value will be used
// as the Envelope's PayloadType. When the Envelope is later unsealed, the PayloadType
// will be used to lookup the correct Record type to unmarshal the Envelope payload into.
func (r Record) Codec() []byte { return EnvelopePayloadType }

// MarshalRecord converts a Record instance to a []byte, so that it can be used as an
// Envelope payload.
func (r Record) MarshalRecord() ([]byte, error) {
	return r.Message().MarshalPacked()
}

// UnmarshalRecord unmarshals a []byte payload into an instance of a particular Record type.
func (r *Record) UnmarshalRecord(b []byte) error {
	m, err := capnp.UnmarshalPacked(b)
	if err == nil {
		*(*boot.Packet)(r), err = boot.ReadRootPacket(m)
	}

	return err
}
