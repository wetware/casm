package pulse

import (
	"time"

	"capnproto.org/go/capnp/v3"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/wetware/casm/internal/api/pulse"
)

type (
	EventType = pulse.Event_Which

	Meta     = pulse.Event_Heartbeat_meta
	MetaType = pulse.Event_Heartbeat_meta_Which
)

var (
	EventType_Heartbeat = pulse.Event_Which_heartbeat
	EventType_Join      = pulse.Event_Which_join
	EventType_Leave     = pulse.Event_Which_leave

	MetaType_None   = pulse.Event_Heartbeat_meta_Which_none
	MetaType_Text   = pulse.Event_Heartbeat_meta_Which_text
	MetaType_Binary = pulse.Event_Heartbeat_meta_Which_binary
	MetaType_Ptr    = pulse.Event_Heartbeat_meta_Which_pointer
)

// EvtMembershipChanged is emitted on the local event bus when
// a peer joins or leaves the cluster.
type EvtMembershipChanged struct {
	pubsub.PeerEvent
	Observer peer.ID
}

func newPeerEvent(id peer.ID, msg *pubsub.Message, ev ClusterEvent) EvtMembershipChanged {
	return EvtMembershipChanged{
		Observer: msg.ReceivedFrom,
		PeerEvent: pubsub.PeerEvent{
			Type: ev.which(),
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

// ClusterEvent is broadcast over the cluster topic.
type ClusterEvent struct{ pe pulse.Event }

func NewClusterEvent(a capnp.Arena) (ClusterEvent, error) {
	var (
		ev        ClusterEvent
		_, s, err = capnp.NewMessage(a)
	)

	if err == nil {
		ev.pe, err = pulse.NewRootEvent(s)
	}

	return ev, err
}

func (ev ClusterEvent) Type() EventType { return ev.pe.Which() }

func (ev ClusterEvent) NewHeartbeat() (Heartbeat, error) {
	h, err := ev.pe.NewHeartbeat()
	return Heartbeat{h}, err
}

func (ev ClusterEvent) Heartbeat() (Heartbeat, error) {
	h, err := ev.pe.Heartbeat()
	return Heartbeat{h}, err
}

func (ev ClusterEvent) Join() (string, error) {
	return ev.pe.Join()
}

func (ev ClusterEvent) SetJoin(id peer.ID) error {
	return ev.pe.SetJoin(string(id))
}

func (ev ClusterEvent) Leave() (string, error) {
	return ev.pe.Leave()
}

func (ev ClusterEvent) SetLeave(id peer.ID) error {
	return ev.pe.SetLeave(string(id))
}

func (ev ClusterEvent) MarshalBinary() ([]byte, error) {
	return ev.pe.Message().MarshalPacked()
}

func (ev *ClusterEvent) UnmarshalBinary(b []byte) error {
	msg, err := capnp.UnmarshalPacked(b)
	if err == nil {
		ev.pe, err = pulse.ReadRootEvent(msg)
	}

	return err
}

type Heartbeat struct{ ph pulse.Event_Heartbeat }

func (hb Heartbeat) SetTTL(d time.Duration) { hb.ph.SetTtl(int64(d)) }
func (hb Heartbeat) TTL() time.Duration     { return time.Duration(hb.ph.Ttl()) }
func (hb Heartbeat) Meta() Meta             { return hb.ph.Meta() }

func (ev ClusterEvent) which() pubsub.EventType {
	switch ev.pe.Which() {
	case pulse.Event_Which_join:
		return pubsub.PeerJoin
	case pulse.Event_Which_leave:
		return pubsub.PeerLeave
	}

	panic("unreachable")
}
