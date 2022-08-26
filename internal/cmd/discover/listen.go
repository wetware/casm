package discover

import (
	"crypto/md5"
	"fmt"
	"text/template"
	"time"

	"github.com/libp2p/go-libp2p/core/record"
	"github.com/muesli/termenv"
	manet "github.com/multiformats/go-multiaddr/net"
	"github.com/urfave/cli/v2"
	"github.com/wetware/casm/pkg/boot/crawl"
	"github.com/wetware/casm/pkg/boot/socket"
	"github.com/wetware/casm/pkg/boot/survey"
)

const templ = `[{{ printf "%04d" .Tick }}] Got {{ .Proto }} packet ({{ .Size }} byte)
  type:      {{ .Type }}
  namespace: {{ .Colorize .Namespace }}
  size:      {{ .Size }} bytes
  peer:	     {{ .Colorize .Peer.String }}
{{- if eq .Type 1 }}  
  distance:   {{ .Distance }}
{{- end }}
{{- with .Err }}
  {{ Color "#cc0000" "ERROR:" }}     {{.}}
{{- end }}

`

var (
	t0 time.Time

	p = termenv.ColorProfile()
	t = template.Must(template.New("boot").
		Funcs(termenv.TemplateFuncs(p)).
		Parse(templ))
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

type templateCtx struct {
	Err   error
	Proto string
	*record.Envelope
}

func (ctx templateCtx) Colorize(s string) termenv.Style {
	hash := md5.Sum([]byte(s))
	hash[0] = hash[0] << 1
	hash[1] = hash[1] << 1
	hash[2] = hash[2] << 1
	color := fmt.Sprintf("#%x", hash[:3])
	return termenv.String(s).Foreground(p.Color(color))
}

func (ctx templateCtx) Size() (int, error) {
	b, err := ctx.Envelope.Marshal()
	return len(b), err
}

func (ctx templateCtx) Tick() time.Duration {
	if t0.IsZero() {
		t0 = time.Now()
	}

	return time.Since(t0) / time.Second
}

type request struct {
	templateCtx
	socket.Request
}

type response struct {
	templateCtx
	socket.Response
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
		case socket.TypeRequest, socket.TypeSurvey:
			return t.Execute(c.App.Writer, request{
				templateCtx: templateCtx{
					Proto:    proto,
					Err:      validate(e, r),
					Envelope: e,
				},
				Request: socket.Request{Record: *r},
			})

		case socket.TypeResponse:
			return t.Execute(c.App.Writer, response{
				templateCtx: templateCtx{
					Proto:    proto,
					Err:      validate(e, r),
					Envelope: e,
				},
				Response: socket.Response{Record: *r},
			})

		default:
			panic(fmt.Errorf("%s", r.Type()))
		}
	}
}
