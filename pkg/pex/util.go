package pex

import (
	"github.com/libp2p/go-libp2p-core/peer"
)

/*
 * utils.go contains unexported utility types
 */

// streamError is a sentinel value used by PeerExchange.gossip.
type streamError struct {
	Peer peer.ID
	error
}

func (err streamError) Loggable() map[string]interface{} {
	return map[string]interface{}{
		"peer":  err.Peer,
		"error": err.error,
	}
}
