package pex

// import (
// 	"sort"

// 	"github.com/libp2p/go-libp2p-core/peer"
// )

// // EvtViewUpdated is emitted on a the host's event bus when
// // a gossip round modifies the current view.
// type EvtViewUpdated []*GossipRecord

// func (ev EvtViewUpdated) Loggable() map[string]interface{} {
// 	ps := make(peer.IDSlice, len(ev))
// 	for i, g := range ev {
// 		ps[i] = g.PeerID
// 	}

// 	// make it a bit easier to read through logs...
// 	sort.Sort(ps)

// 	return map[string]interface{}{
// 		"pex_view": ps,
// 	}
// }
