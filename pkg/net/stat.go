package net

// import (
// 	"sync/atomic"

// 	"github.com/libp2p/go-libp2p-core/peer"
// 	"github.com/lthibault/log"
// )

// const (
// 	statReady   = stateStat(StateDisconnected)
// 	statClosing = stateStat(StateClosing)
// 	statClosed  = stateStat(StateClosed)
// )

// // Stat contains metadata about the overlay network.
// type Stat interface {
// 	log.Loggable
// 	State() State
// 	Peer() peer.ID
// 	View() []peer.AddrInfo
// }

// type stat struct{ *EvtStateChanged }

// func (s stat) Peer() peer.ID { return s.EvtStateChanged.Peer }

// type stateStat State

// func (s stateStat) Loggable() map[string]interface{} {
// 	return map[string]interface{}{
// 		"state": State(s),
// 	}
// }
// func (s stateStat) State() State        { return State(s) }
// func (stateStat) Peer() peer.ID         { return "" }
// func (stateStat) View() []peer.AddrInfo { return nil }

// type statStore atomic.Value

// func (ss *statStore) Load() Stat   { return (*atomic.Value)(ss).Load().(Stat) }
// func (ss *statStore) Store(s Stat) { (*atomic.Value)(ss).Store(s) }
