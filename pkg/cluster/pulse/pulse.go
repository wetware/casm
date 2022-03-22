//go:generate mockgen -source=pulse.go -destination=../../../internal/mock/pkg/cluster/pulse/pulse.go -package=mock_pulse

// Package pulse provides ev cluster-management service based on pubsub.
package pulse

import (
	"context"
	"encoding/binary"

	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/wetware/casm/pkg/cluster/routing"
)

type RoutingTable interface {
	Upsert(routing.Record) bool
}

type Preparer interface {
	Prepare(Heartbeat)
}

func NewValidator(rt RoutingTable) pubsub.ValidatorEx {
	return func(_ context.Context, id peer.ID, m *pubsub.Message) pubsub.ValidationResult {
		var hb Heartbeat
		if err := hb.UnmarshalBinary(m.Data); err == nil {
			m.ValidatorData = hb

			if rt.Upsert(record(m, hb)) {
				return pubsub.ValidationAccept
			}

			// heartbeat is valid, but we have a more recent one.
			return pubsub.ValidationIgnore
		}

		// assume the worst...
		return pubsub.ValidationReject
	}
}

type routingRecord struct {
	m *pubsub.Message
	Heartbeat
}

func (r routingRecord) Peer() peer.ID { return r.m.GetFrom() }
func (r routingRecord) Seq() uint64   { return binary.BigEndian.Uint64(r.m.GetSeqno()) }

func record(m *pubsub.Message, h Heartbeat) routingRecord {
	return routingRecord{
		m:         m,
		Heartbeat: h,
	}
}
