package pex

import (
	"github.com/libp2p/go-libp2p-core/peer"
)

/*
 * utils.go contains unexported utility types
 */

// breaker is a helper type that is used to elide repeated
// error checks.  Calls to Do() become a nop when Err != nil.
//
// See:  https://go.dev/blog/errors-are-values
type breaker struct{ Err error }

func (b *breaker) Do(f func()) {
	if b.Err == nil {
		f()
	}
}

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
