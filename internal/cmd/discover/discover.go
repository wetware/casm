package discover

import (
	"fmt"
	"net"
	"os/signal"
	"syscall"
	"text/template"

	"github.com/lthibault/log"
	"github.com/muesli/termenv"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/urfave/cli/v2"
	logutil "github.com/wetware/casm/internal/util/log"
	"github.com/wetware/casm/pkg/boot/crawl"
	"github.com/wetware/casm/pkg/boot/socket"
	"github.com/wetware/casm/pkg/boot/survey"
)

const templ = `Got {{ .Proto }} packet ({{ .Size }} byte)
  type:      {{ .Type }}
  namespace: {{ .Colorize .Namespace }}
  size:      {{ .Size }} bytes
  peer:	     {{ .Colorize .PeerID.String }}
{{- if eq .Type 1 }}  
  distance:   {{ .Distance }}
{{- end }}
{{- with .Err }}
  {{ Color "#cc0000" "ERROR:" }}     {{.}}
{{- end }}

`

var (
	p = termenv.ColorProfile()
	t = template.Must(template.New("boot").
		Funcs(termenv.TemplateFuncs(p)).
		Parse(templ))
)

var (
	sock   *socket.Socket
	maddr  ma.Multiaddr
	logger log.Logger
	addr   *net.UDPAddr
	ifi    *net.Interface
)

var flags = []cli.Flag{
	&cli.StringFlag{
		Name:    "ns",
		Usage:   "cluster namespace",
		Value:   "casm",
		EnvVars: []string{"CASM_NS"},
	},
	&cli.StringFlag{
		Name:    "addr",
		Aliases: []string{"a"},
		Usage:   "discovery service multiaddress",
		Value:   "/ip4/228.8.8.8/udp/8822/multicast/lo0",
	},
}

var commands = []*cli.Command{
	listen(),
	emit(),
	genpayload(),
}

// Command constructor
func Command() *cli.Command {
	return &cli.Command{
		Name:        "discover",
		Usage:       "discover peers on the network",
		Flags:       flags,
		Subcommands: commands,
		Before:      parse(),
		After:       teardown(),
	}
}

func parse() cli.BeforeFunc {
	return func(c *cli.Context) (err error) {
		logger = logutil.New(c)

		maddr, err = ma.NewMultiaddr(c.String("addr"))
		return
	}
}

func teardown() cli.AfterFunc {
	return func(c *cli.Context) error {
		if sock != nil {
			return sock.Close()
		}

		return nil
	}
}

func setsock(c *cli.Context, conn net.PacketConn) error {
	sock = socket.New(conn, nil, socket.Protocol{
		HandleError:   errlogger(c),
		HandleRequest: func(socket.Request, net.Addr) {},
		Validate:      render(c, "multicast"),
	})

	return nil
}

func errlogger(c *cli.Context) func(error) {
	ctx, cancel := signal.NotifyContext(c.Context,
		syscall.SIGINT,
		syscall.SIGTERM)
	defer cancel()

	const red = "#cc0000"
	emsg := termenv.String("ERROR").Foreground(p.Color(red))

	return func(err error) {
		if ctx.Err() == nil {
			fmt.Fprintf(c.App.Writer, "%s: %s\n", emsg, err)
		}
	}
}

func task(f cli.ActionFunc, c *cli.Context) func() error {
	return func() error {
		return f(c)
	}
}

func proto() (proto int) {
	ma.ForEach(maddr, func(c ma.Component) bool {
		switch proto = c.Protocol().Code; proto {
		case crawl.P_CIDR, survey.P_MULTICAST:
			return false
		}

		return true
	})

	return
}
