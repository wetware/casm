package pex

import "github.com/libp2p/go-libp2p-core/discovery"

/*
 * discovery.go contains a background discovery service based on the
 * implementation found in go-libp2p-pubsub, which bootstraps namespace
 * connectivity.
 */

type discover struct {
	d   discovery.Discovery
	opt []discovery.Option
}
