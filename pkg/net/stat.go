package net

import (
	"sync/atomic"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/lthibault/log"
)

const (
	statReady   = stateStat(StateDisconnected)
	statClosing = stateStat(StateClosing)
	statClosed  = stateStat(StateClosed)
)

// Stat contains metadata about the overlay network.
type Stat interface {
	log.Loggable
	State() State
	Peer() peer.ID
	Event() Event
	View() Slots
}

type stat struct{ *EvtNetState }

func (s stat) Peer() peer.ID { return s.EvtNetState.Peer }
func (s stat) Event() Event  { return s.EvtNetState.Event }

type stateStat State

func (s stateStat) Loggable() map[string]interface{} {
	return map[string]interface{}{
		"state": State(s),
	}
}
func (s stateStat) State() State { return State(s) }
func (stateStat) Peer() peer.ID  { return "" }
func (stateStat) Event() Event   { return eventNull }
func (stateStat) View() Slots    { return nil }

type statStore atomic.Value

func (ss *statStore) Load() Stat   { return (*atomic.Value)(ss).Load().(Stat) }
func (ss *statStore) Store(s Stat) { (*atomic.Value)(ss).Store(s) }
