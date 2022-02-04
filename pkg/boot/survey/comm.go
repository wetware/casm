package survey

import (
	"context"
	"net"

	"capnproto.org/go/capnp/v3"
)

type comm struct {
	cherr chan<- error
	recv  chan<- *capnp.Message
	send  <-chan *capnp.Message

	addr  net.Addr
	dconn net.PacketConn
}

func (c *comm) Send(ctx context.Context, m *capnp.Message) (err error) {
	select {
	case c.recv <- m:
	case <-ctx.Done():
		err = ctx.Err()
	}

	return
}

func (c *comm) StartRecv(ctx context.Context, conn net.PacketConn) {
	defer conn.Close()
	var buf [maxDatagramSize]byte

	for {
		n, _, err := conn.ReadFrom(buf[:])
		if err != nil {
			if e, ok := err.(net.Error); ok && e.Temporary() {
				continue // TODO:  exponential backoff + logging
			}

			select {
			case c.cherr <- err:
			default:
			}

			return
		}

		m, err := capnp.UnmarshalPacked(buf[:n])
		if err != nil {
			continue // TODO:  log
		}

		select {
		case c.recv <- m:
		case <-ctx.Done():
			return
		}
	}
}

func (c *comm) StartSend(ctx context.Context, conn net.PacketConn) {
	defer conn.Close()

	for {
		select {
		case m := <-c.send:
			b, err := m.MarshalPacked()
			if err != nil {
				panic(err)
			}

			if _, err = conn.WriteTo(b, c.addr); err != nil {
				if e, ok := err.(net.Error); ok && e.Temporary() {
					continue // TODO:  exponential backoff + logging
				}

				select {
				case c.cherr <- err:
				default:
				}

				return
			}

		case <-ctx.Done():
		}
	}

}
