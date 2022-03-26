package discover

import (
	"crypto/md5"
	"fmt"

	"github.com/libp2p/go-libp2p-core/record"
	"github.com/muesli/termenv"
	manet "github.com/multiformats/go-multiaddr/net"
	"github.com/urfave/cli/v2"
	"github.com/wetware/casm/pkg/boot/crawl"
	"github.com/wetware/casm/pkg/boot/socket"
	"github.com/wetware/casm/pkg/boot/survey"
)

func listen() *cli.Command {
	return &cli.Command{
		Name:   "listen",
		Usage:  "listen for incoming request packets",
		Before: bindsock(),
		Action: recv,
	}
}

func bindsock() cli.BeforeFunc {
	return func(c *cli.Context) (err error) {
		switch proto() {
		case crawl.P_CIDR:
			return unicast(c)

		case survey.P_MULTICAST:
			if addr, ifi, err = survey.ResolveMulticast(maddr); err != nil {
				return
			}

			logger = logger.WithField("group", addr)
			return multicast(c)

		default:
			return fmt.Errorf("unknown protocol")
		}
	}
}

func unicast(c *cli.Context) error {
	conn, err := manet.ListenPacket(maddr)
	if err != nil {
		return err
	}

	return setsock(c, conn)
}

func multicast(c *cli.Context) error {
	conn, err := survey.JoinMulticastGroup("udp4", ifi, addr)
	if err != nil {
		return fmt.Errorf("join multicast: %w", err)
	}

	return setsock(c, conn)
}

func recv(c *cli.Context) error {
	<-c.Done()
	return nil
}

type request struct {
	Proto string
	Err   error
	socket.Request
	*record.Envelope
}

func (r request) Colorize(s string) termenv.Style {
	return colorID(s)
}

func (r request) Size() (int, error) {
	b, err := r.Envelope.Marshal()
	return len(b), err
}

type response struct {
	Proto string
	Err   error
	socket.Response
	*record.Envelope
}

func (r response) Colorize(s string) termenv.Style {
	return colorID(s)
}

func (r response) Size() (int, error) {
	b, err := r.Envelope.Marshal()
	return len(b), err
}

func colorID(s string) termenv.Style {
	hash := md5.Sum([]byte(s))
	hash[0] <<= 1 // make sure it's not too dark
	hash[1] <<= 1
	hash[3] <<= 1
	color := fmt.Sprintf("#%x", hash[:3])
	return termenv.String(s).Foreground(p.Color(color))
}

func render(c *cli.Context, proto string) func(*record.Envelope, *socket.Record) error {
	if c.Command.Name == "emit" {
		return func(*record.Envelope, *socket.Record) error {
			return nil
		}
	}

	validate := socket.BasicValidator("")

	return func(e *record.Envelope, r *socket.Record) (err error) {
		switch r.Type() {
		case socket.TypeRequest, socket.TypeGradualRequest:
			return t.Execute(c.App.Writer, request{
				Proto:    proto,
				Err:      validate(e, r),
				Request:  socket.Request{Record: *r},
				Envelope: e,
			})

		default:
			return t.Execute(c.App.Writer, response{
				Proto:    proto,
				Err:      validate(e, r),
				Response: socket.Response{Record: *r},
				Envelope: e,
			})
		}

	}
}
