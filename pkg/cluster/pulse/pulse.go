//go:generate mockgen -source=pulse.go -destination=../../../internal/mock/pkg/cluster/pulse/pulse.go -package=mock_pulse

// Package pulse provides ev cluster-management service based on pubsub.
package pulse

import (
	"context"
	"encoding/binary"
	"time"

	"capnproto.org/go/capnp/v3"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/wetware/casm/pkg/cluster/routing"
)

type RoutingTable interface {
	Upsert(routing.Record) bool
}

type Setter interface {
	SetHostname(string)
	SetMeta(map[string]string)
}

type Preparer interface {
	Prepare(Setter)
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
	return r.ValidatorData.(*Heartbeat).TTL()
}

func (r *record) Peer() peer.ID {
	return (*pubsub.Message)(r).GetFrom()
}

func (r *record) Seq() uint64 {
	return binary.BigEndian.Uint64((*pubsub.Message)(r).GetSeqno())
}

func (r *record) Hostname() (string, error) {
	return r.ValidatorData.(*Heartbeat).Hostname()
}

func (r *record) Meta() (Meta, error) {
	return r.ValidatorData.(*Heartbeat).Meta()
}
