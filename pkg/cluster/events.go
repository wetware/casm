package cluster

import (
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/wetware/casm/internal/api/cluster"
)

// EvtMembershipChanged is emitted when a peer joins or leaves the cluster.
type EvtMembershipChanged struct {
	pubsub.PeerEvent
	Observer peer.ID
}

func newPeerEvent(id peer.ID, msg *pubsub.Message, a announcement) EvtMembershipChanged {
	return EvtMembershipChanged{
		Observer: msg.ReceivedFrom,
		PeerEvent: pubsub.PeerEvent{
			Type: evtype(a),
			Peer: id,
		},
	}
}

func (ev EvtMembershipChanged) Loggable() map[string]interface{} {
	return map[string]interface{}{
		"event":    ev.String(),
		"peer_id":  ev.Peer,
		"observer": ev.Observer,
	}
}

func (ev EvtMembershipChanged) String() string {
	switch ev.PeerEvent.Type {
	case pubsub.PeerJoin:
		return "join"
	case pubsub.PeerLeave:
		return "leave"
	}

	panic("unreachable")
}

func evtype(a announcement) pubsub.EventType {
	switch a.Which() {
	case cluster.Announcement_Which_join:
		return pubsub.PeerJoin
	case cluster.Announcement_Which_leave:
		return pubsub.PeerLeave
	}

	panic("unreachable")
}
