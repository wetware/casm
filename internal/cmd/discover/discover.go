package discover

import (
	"fmt"
	"io/ioutil"
	"net"
	"text/template"

	"github.com/libp2p/go-libp2p-core/record"
	"github.com/lthibault/log"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/urfave/cli/v2"
	logutil "github.com/wetware/casm/internal/util/log"
	"github.com/wetware/casm/pkg/boot/crawl"
	"github.com/wetware/casm/pkg/boot/socket"
	"github.com/wetware/casm/pkg/boot/survey"
	"golang.org/x/sync/errgroup"
)

const templ = `{{.Type}} from {{.From}}`

var t = template.Must(template.New("boot").Parse(templ))

var (
	sock   *socket.Socket
	sync   = make(chan struct{})
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
		Name:    "discover",
		Aliases: []string{"d"},
		Usage:   "discovery service multiaddress",
		Value:   "/ip4/228.8.8.8/udp/8822/multicast/lo0",
	},
	&cli.BoolFlag{
		Name:    "emit",
		Aliases: []string{"e"},
		Usage:   "emit auto-generated discovery packet",
	},
}

var commands = []*cli.Command{
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
		After:       shutdown(),
		Action:      discover(),
	}
}

func parse() cli.BeforeFunc {
	return func(c *cli.Context) (err error) {
		logger = logutil.New(c)

		if maddr, err = ma.NewMultiaddr(c.String("discover")); err != nil {
			return
		}

		switch proto() {
		case crawl.P_CIDR:
			return fmt.Errorf("NOT IMPLEMENTED")

		case survey.P_MULTICAST:
			if addr, ifi, err = survey.ResolveMulticast(maddr); err == nil {
				logger = logger.WithField("group", addr)
			}

		default:
			return fmt.Errorf("unknown protocol")
		}

		return
	}
}

func shutdown() cli.AfterFunc {
	return func(ctx *cli.Context) (err error) {
		if sock != nil {
			err = sock.Close()
		}

		return
	}
}

func discover() cli.ActionFunc {
	return func(c *cli.Context) error {
		var g *errgroup.Group
		g, c.Context = errgroup.WithContext(c.Context)

		g.Go(listen(c))
		g.Go(dial(c))

		return g.Wait()
	}
}

func listen(c *cli.Context) func() error {
	return func() error {
		switch proto() {
		case crawl.P_CIDR:
			return unicast(c)

		case survey.P_MULTICAST:
			return multicast(c)

		default:
			return fmt.Errorf("unknown protocol")
		}
	}
}

func dial(c *cli.Context) func() error {
	return func() error {
		e, err := payload(c)
		if err != nil {
			return err
		}

		select {
		case <-sync:
		case <-c.Done():
			return c.Err()
		}

		if err = sock.Send(e, addr); err == nil {
			logger.Info("send payload")
		}

		return err
	}
}

func payload(c *cli.Context) (*record.Envelope, error) {
	if c.IsSet("emit") {
		return generate(c)
	}

	// Read signed envelope at the command line
	b, err := ioutil.ReadAll(c.App.Reader)
	if err != nil {
		return nil, err
	}

	return record.UnmarshalEnvelope(b)
}

func unicast(c *cli.Context) error {
	return fmt.Errorf("NOT IMPLEMENTED")
}

func multicast(c *cli.Context) error {
	conn, err := survey.JoinMulticastGroup(addr.Network(), ifi, addr)
	if err != nil {
		return fmt.Errorf("join multicast: %w", err)
	}
	defer conn.Close()

	logger.Infof("listening for multicast traffic on interface %s", ifi.Name)

	sock = socket.New(conn, socket.Protocol{
		HandleError:   onError,
		HandleRequest: func(socket.Request, net.Addr) {},
		Validate:      render(c),
	})

	close(sync)
	<-c.Done()

	return nil
}

func render(c *cli.Context) func(*record.Envelope, *socket.Record) error {
	return func(e *record.Envelope, r *socket.Record) error {
		return t.Execute(c.App.Writer, r)
	}
}

func onError(err error) {
	logger.WithError(err).Error("socket error")
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
