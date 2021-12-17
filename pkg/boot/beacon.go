package boot

import (
	"bytes"
	"context"
	"io"
	"net"
	"sync/atomic"
	"time"

	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/host"
	ps "github.com/libp2p/go-libp2p-core/peerstore"
)

// Beacon is a small discovery server that binds to a local address
// and replies to incoming connections with the Host's peer record.
type Beacon struct {
	Addr net.Addr
	Host host.Host
}

func (b Beacon) Advertise(ctx context.Context, ns string, opt ...discovery.Option) (time.Duration, error) {
	// TODO
	return ps.PermanentAddrTTL, nil
}

func (b Beacon) Serve(ctx context.Context) error {
	sub, err := b.Host.EventBus().Subscribe(new(event.EvtLocalAddressesUpdated))
	if err != nil {
		return err
	}
	defer sub.Close()

	server, err := new(net.ListenConfig).Listen(ctx,
		b.Addr.Network(),
		b.Addr.String())
	if err != nil {
		return err
	}
	defer server.Close()

	var payload atomic.Value
	requests := make(chan net.Conn, 1)
	defer close(requests)

	go func() {
		for v := range sub.Out() {
			ev := v.(event.EvtLocalAddressesUpdated)
			b, err := ev.SignedPeerRecord.Marshal()
			if err != nil {
				return
			}

			payload.Store(b)
		}
	}()

	for ctx.Err() == nil {
		conn, err := server.Accept()
		if err != nil {
			return err
		}

		go b.handle(conn, &payload)

	}

	return ctx.Err()
}

func (b Beacon) handle(conn net.Conn, record *atomic.Value) {
	if err := conn.SetWriteDeadline(time.Now().Add(time.Second)); err == nil {
		io.Copy(conn, bytes.NewReader(record.Load().([]byte)))
	}
}
