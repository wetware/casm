// Package pulse provides a pubsub-based peer-clustering and membership service.
package pulse

import (
	"context"
	"encoding/binary"

	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/wetware/casm/internal/api/pulse"
	"github.com/wetware/casm/pkg/routing"
)

/*
 • join cluster(cluster configuration): function for joining a cluster on top of a peer-to-peer network.
                                        Note that the cluster configuration parameter depends on each
                                        implementation. For example, in the case of T-Man, it may include
										a ranking function.

 • leave cluster(cluster id): function for gracefully leaving a cluster.

 • get neighbours(): function for explicitly retrieving neighbours from the local view.

 • set neighbours callback(callback): function for passing a callback to the clustering protocol,
                                      which will be called whenever the biased neighbourhood changes.
*/

type RoutingTable interface {
	Upsert(peer.ID, routing.Record) bool
}

func NewValidator(t RoutingTable, e event.Emitter) pubsub.ValidatorEx {
	return func(_ context.Context, id peer.ID, msg *pubsub.Message) pubsub.ValidationResult {
		var a announcement
		if err := a.UnmarshalBinary(msg.Data); err != nil {
			return pubsub.ValidationReject
		}

		msg.ValidatorData = a

		switch a.Which() {
		case pulse.Announcement_Which_heartbeat:
			if hb, err := a.Heartbeat(); err == nil {
				if t.Upsert(id, record(msg, hb)) {
					return pubsub.ValidationAccept
				}

				// heartbeat is valid, but we have a more recent one.
				return pubsub.ValidationIgnore
			}

		case pulse.Announcement_Which_join, pulse.Announcement_Which_leave:
			if validateJoinLeaveID(a) {
				_ = e.Emit(newPeerEvent(id, msg, a))
				return pubsub.ValidationAccept
			}
		}

		// assume the worst...
		return pubsub.ValidationReject
	}
}
func seqno(msg *pubsub.Message) uint64 {
	return binary.BigEndian.Uint64(msg.GetSeqno())
}

func validateJoinLeaveID(a announcement) bool {
	var (
		s   string
		err error
	)

	switch a.Which() {
	case pulse.Announcement_Which_join:
		s, err = a.Join()
	case pulse.Announcement_Which_leave:
		s, err = a.Leave()
	}

	return err == nil
}
