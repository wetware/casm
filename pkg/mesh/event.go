package mesh

import (
	"fmt"

	"github.com/libp2p/go-libp2p-core/peer"
)

const (
	// EventJoined indicates a peer has joined the neighborhood.
	EventJoined Event = iota

	// EventLeft indicates a peer has left the neighborhood.
	EventLeft

	// StateEmpty indicates the neighborhood contains 0 peers.
	StateEmpty State = iota

	// StatePartial indicates the neighborhood contains 0 < n < k-1 peers.
	StatePartial

	// StateFull indicates the neighborhood contains k peers.
	StateFull

	// StateClosed indicates that the neighborhood is no longer accepting
	// peer connections.
	StateClosed
)

type EvtNeighborhoodChanged struct {
	Peer  peer.ID
	Event Event
	State State
	View  peer.IDSlice
}

func (ev EvtNeighborhoodChanged) Loggable() map[string]interface{} {
	return map[string]interface{}{
		"peer":  ev.Peer.String(),
		"event": ev.Event.String(),
		"state": ev.State.String(),
	}
}

func (ev EvtNeighborhoodChanged) String() string {
	return fmt.Sprintf("%s %s", ev.Peer, ev.Event)
}

// An Event represents a state transition in a neighborhood.
type Event uint8

func (e Event) Loggable() map[string]interface{} {
	return map[string]interface{}{
		"event": e.String(),
	}
}

func (e Event) String() string {
	switch e {
	case EventJoined:
		return "joined"
	case EventLeft:
		return "left"
	}

	panic(fmt.Sprintf("invalid event '%d'", e))
}

// State of the neighborhood
type State uint8

func (s State) Loggable() map[string]interface{} {
	return map[string]interface{}{
		"state": s.String(),
	}
}

func (s State) String() string {
	switch s {
	case StateEmpty:
		return "empty"
	case StatePartial:
		return "partial"
	case StateFull:
		return "full"
	case StateClosed:
		return "closing"
	default:
		panic(fmt.Sprintf("invalid state '%d'", s))
	}
}

func state(slots peer.IDSlice) State {
	switch {
	case len(slots) == 0:
		return StateEmpty
	case isFull(slots):
		return StateFull
	default:
		return StatePartial
	}
}
