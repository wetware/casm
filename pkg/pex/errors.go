package pex

import "errors"

var (
	GossipRunningError     = errors.New("gossip engine is already running")
	GossipNoRunningError   = errors.New("gossip engine is not running")
	DuplicateGossiperError = errors.New("gossiper with same ID is already registered")
	GossiperNotFoundError  = errors.New("gossiper was not found") // TODO: specify gossiper ID
)

type GossipStoppedError struct{}

func (e *GossipStoppedError) Error() string {
	return "gossiper is stopped"
}

type NoPeersError struct{}

func (e *NoPeersError) Error() string {
	return "there are no peers to gossip with"
}
