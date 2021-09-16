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

type syncChan chan struct{}

func (ch syncChan) Signal() { ch <- struct{}{} }

type syncChanPool chan chan struct{}

var chSyncPool = make(syncChanPool, 8)

func (pool syncChanPool) Get() (ch syncChan) {
	select {
	case ch = <-pool:
	default:
		ch = make(syncChan, 1)
	}

	return
}

func (pool syncChanPool) Put(ch syncChan) {
	select {
	case <-ch:
	default:
	}

	select {
	case pool <- ch:
	default:
	}
}
