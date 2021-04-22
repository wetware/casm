// Package cluster implements a clustering service on top of an overlay network.
package cluster

import (
	"context"
	"io"

	"github.com/jbenet/goprocess"
	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/lthibault/log"
	"github.com/wetware/casm/pkg/net"
)

var _ Network = (*net.Overlay)(nil)

type Network interface {
	log.Loggable
	io.Closer
	discovery.Discoverer

	Join(context.Context, discovery.Discoverer, ...discovery.Option) error
	Stat() peer.IDSlice
	Host() host.Host
	Process() goprocess.Process
}

type Cluster struct {
	log log.Logger
	net Network
}

// New cluster.
func New(net Network, opt ...Option) (*Cluster, error) {
	var c = &Cluster{net: net}
	for _, option := range withDefaults(opt) {
		option(c)
	}

	return c, nil
}
