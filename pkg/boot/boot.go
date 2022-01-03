// Package boot provides cluster bootstrapping primitives.
package boot

import (
	"context"
	"net"

	"github.com/libp2p/go-libp2p-core/record"
)

type Dialer interface {
	DialContext(ctx context.Context, network, addr string) (net.Conn, error)
}

type DialStrategy interface {
	Dial(context.Context, Dialer) (<-chan net.Conn, error)
}

type Scanner interface {
	Scan(net.Conn, record.Record) (*record.Envelope, error)
}
