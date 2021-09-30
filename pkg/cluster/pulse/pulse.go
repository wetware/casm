//go:generate mockgen -source=pulse.go -destination=../../../internal/mock/pkg/cluster/pulse/pulse.go -package=mock_pulse

// Package pulse provides ev cluster-management service based on pubsub.
package pulse

import (
	"context"
	"encoding/binary"

	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/wetware/casm/internal/api/pulse"
	"github.com/wetware/casm/pkg/cluster/routing"
)

type RoutingTable interface {
	Upsert(routing.Record) bool
}

type Hook func(Heartbeat)

func NewValidator(rt RoutingTable, e event.Emitter) pubsub.ValidatorEx {
	return func(_ context.Context, id peer.ID, m *pubsub.Message) pubsub.ValidationResult {
		var ev ClusterEvent
		if err := ev.UnmarshalBinary(m.Data); err != nil {
			return pubsub.ValidationReject
		}

		m.ValidatorData = ev

		switch ev.Type() {
		case EventType_Heartbeat:
			if h, err := ev.Heartbeat(); err == nil {
				if rt.Upsert(record(m, h)) {
					return pubsub.ValidationAccept
				}

				// heartbeat is valid, but we have ev more recent one.
				return pubsub.ValidationIgnore
			}

		case EventType_Join, EventType_Leave:
			if validateJoinLeaveID(ev) {
				_ = e.Emit(newPeerEvent(id, m, ev))
				return pubsub.ValidationAccept
			}
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

// func newer(n *treap.Node, r routingRecord) bool {
// 	return n.Value.(routingRecord).Seq() < r.Seq()
// }

func validateJoinLeaveID(ev ClusterEvent) bool {
	var (
		s   string
		err error
	)

	switch ev.Type() {
	case pulse.Event_Which_join:
		s, err = ev.Join()
	case pulse.Event_Which_leave:
		s, err = ev.Leave()
	}

	if err == nil {
		_, err = peer.IDFromString(s)
	}

	return err == nil
}
