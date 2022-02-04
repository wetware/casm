package survey

import (
	"context"
	"net"
	"time"

	"capnproto.org/go/capnp/v3"
	"github.com/jpillora/backoff"
	"github.com/lthibault/log"
)

// comm handles network communication
type comm struct {
	log log.Logger

	cherr chan<- error
	recv  chan<- *capnp.Message
	send  <-chan *capnp.Message

	addr  net.Addr
	dconn net.PacketConn
}

func (c comm) Send(ctx context.Context, m *capnp.Message) (err error) {
	select {
	case c.recv <- m:
	case <-ctx.Done():
		err = ctx.Err()
	}

	return
}

func (c comm) StartRecv(ctx context.Context, conn net.PacketConn) {
	var buf [maxDatagramSize]byte

	b := backoff.Backoff{
		Factor: 2,
		Jitter: true,
		Min:    time.Second,
		Max:    time.Minute * 15,
	}

	for {
		n, _, err := conn.ReadFrom(buf[:])
		if err != nil {
			if e, ok := err.(net.Error); ok && e.Temporary() {
				c.log.
					WithError(err).
					WithField("backoff", b.ForAttempt(b.Attempt())).
					Error("read failed")

				select {
				case <-time.After(b.Duration()):
				case <-ctx.Done():
				}

				continue
			}

			select {
			case c.cherr <- err:
			default:
			}

			return
		}

		b.Reset()

		m, err := capnp.UnmarshalPacked(buf[:n])
		if err != nil {
			panic(err)
		}

		select {
		case c.recv <- m:
		case <-ctx.Done():
			return
		}
	}
}

func (c comm) StartSend(ctx context.Context, conn net.PacketConn) {
	bo := backoff.Backoff{
		Factor: 2,
		Jitter: true,
		Min:    time.Second,
		Max:    time.Minute * 15,
	}

	for {
		select {
		case m := <-c.send:
			b, err := m.MarshalPacked()
			if err != nil {
				panic(err)
			}

			if _, err = conn.WriteTo(b, c.addr); err != nil {
				if e, ok := err.(net.Error); ok && e.Temporary() {
					c.log.
						WithError(err).
						WithField("backoff", bo.ForAttempt(bo.Attempt())).
						Error("read failed")

					select {
					case <-time.After(bo.Duration()):
					case <-ctx.Done():
					}

					continue // TODO:  exponential backoff + logging
				}

				select {
				case c.cherr <- err:
				default:
				}

				return
			}

			bo.Reset()

		case <-ctx.Done():
			return
		}
	}

}
