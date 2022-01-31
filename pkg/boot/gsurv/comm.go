package mudp

import (
	"net"
)

type comm struct {
	addr         net.Addr
	lconn, dconn net.PacketConn
	cherr        chan error
}

func (c *comm) Listen(handle func(int, net.Addr, []byte)) {
	var (
		n    int
		addr net.Addr
		err  error
		b    []byte
	)

	for {
		b = make([]byte, maxDatagramSize)
		n, addr, err = c.lconn.ReadFrom(b)
		if err != nil {
			if e, ok := err.(net.Error); ok && !e.Temporary() {
				select {
				case c.cherr <- err:
				default:
				}
				return
			}
		} else {
			handle(n, addr, b)
		}

	}
}

func (c *comm) Send(b []byte) {
	_, err := c.dconn.WriteTo(b, c.addr)
	if err != nil {
		if e, ok := err.(net.Error); ok && !e.Temporary() {
			select {
			case c.cherr <- err:
			default:
			}
			return
		}
	}
}

func (c *comm) Close() {
	select {
	case c.cherr <- nil:
	default:
	}
}
