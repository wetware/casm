package pex

import (
	"context"

	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/peer"
)

/*
 * discovery.go contains a background discovery service based on the
 * implementation found in go-libp2p-pubsub, which bootstraps namespace
 * connectivity.
 */

type discover struct {
	d   discovery.Discovery
	opt []discovery.Option
}

func (disc discover) StopTracking(ns string) {
	panic("NOT IMPLEMENTED") // XXX
}

// Bootstrap returns peers from the bootstrap discovery service.
func (disc discover) Bootstrap(ctx context.Context, ns string) (<-chan peer.AddrInfo, error) {
	// TODO:  start annouce loop for ns, idempotently.

	return disc.d.FindPeers(ctx, ns, disc.opt...)
}
