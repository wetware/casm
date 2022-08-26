//go:generate mockgen -source=pulse.go -destination=../../../internal/mock/pkg/cluster/pulse/pulse.go -package=mock_pulse

// Package pulse provides ev cluster-management service based on pubsub.
package pulse

import (
	"context"
	"encoding/binary"
	"time"

	"capnproto.org/go/capnp/v3"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"

	api "github.com/wetware/casm/internal/api/routing"
	"github.com/wetware/casm/pkg/cluster/routing"
)

type RoutingTable interface {
	Upsert(routing.Record) bool
}

type Setter interface {
	SetMeta(map[string]string) error
}

type Preparer interface {
	Prepare(Setter) error
}

func NewValidator(rt RoutingTable) pubsub.ValidatorEx {
	return func(_ context.Context, id peer.ID, m *pubsub.Message) pubsub.ValidationResult {
		if rec, err := bind(m); err == nil {
			if rt.Upsert(rec) {
				return pubsub.ValidationAccept
			}

			// heartbeat is valid, but we have a more recent one.
			return pubsub.ValidationIgnore
		}

		// assume the worst...
		return pubsub.ValidationReject
	}
}

type record pubsub.Message

func bind(msg *pubsub.Message) (*record, error) {
	var h Heartbeat
	m, err := capnp.UnmarshalPacked(msg.Data)
	if err == nil {
		msg.ValidatorData = &h
		err = h.ReadMessage(m)
	}

	return (*record)(msg), err
}

func (r *record) TTL() time.Duration {
	return r.heartbeat().TTL()
}

func (r *record) Peer() peer.ID {
	return (*pubsub.Message)(r).GetFrom()
}

func (r *record) PeerBytes() ([]byte, error) {
	return (*pubsub.Message)(r).From, nil
}

func (r *record) Seq() uint64 {
	return binary.BigEndian.Uint64((*pubsub.Message)(r).GetSeqno())
}

func (r *record) Instance() uint32 {
	return r.heartbeat().Instance()
}

func (r *record) Host() (string, error) {
	return r.heartbeat().Host()
}

func (r *record) HostBytes() ([]byte, error) {
	return r.heartbeat().HostBytes()
}

func (r *record) Meta() (routing.Meta, error) {
	return r.heartbeat().Meta()
}

func (r *record) heartbeat() *Heartbeat {
	return r.ValidatorData.(*Heartbeat)
}

func (r *record) BindRecord(rec api.View_Record) (err error) {
	if err = rec.SetPeer(string(r.Peer())); err == nil {
		rec.SetSeq(r.Seq())
		err = rec.SetHeartbeat(r.heartbeat().Heartbeat)
	}

	return
}
