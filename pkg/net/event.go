package net

import (
	"fmt"

	"github.com/libp2p/go-libp2p-core/peer"
)

const (
	eventNull Event = iota

	// EventJoined indicates a peer has joined the neighborhood.
	EventJoined

	// EventLeft indicates a peer has left the neighborhood.
	EventLeft

	// StateDisconnected indicates a peer is not connected to the overlay.
	StateDisconnected State = iota

	// StateConnected indicates a peer is connected to the overlay.
	StateConnected

	// StateClosing indicates the peer is leaving the overlay, and no longer
	// accepting connections.
	StateClosing

	// StateClosed indicates that the peer has left the overlay.
	StateClosed
)

// EvtNetState is an event signalling a change in the state of the
// overlay network.
type EvtNetState struct {
	Event Event
	Peer  peer.ID
	Slots
}

func (ev EvtNetState) Loggable() map[string]interface{} {
	return map[string]interface{}{
		"peer":  ev.Peer,
		"event": ev.Event,
		"state": ev.State(),
	}
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
	case eventNull:
		return "null event"
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
	case StateDisconnected:
		return "disconnected"
	case StateConnected:
		return "connected"
	case StateClosing:
		return "closing"
	case StateClosed:
		return "closed"
	default:
		panic(fmt.Sprintf("invalid state '%d'", s))
	}
}
