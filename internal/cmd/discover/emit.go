package discover

import (
	"fmt"
	"io/ioutil"
	"net"
	"time"

	"github.com/mikelsr/go-libp2p/core/record"
	manet "github.com/multiformats/go-multiaddr/net"
	"github.com/urfave/cli/v2"
	"github.com/wetware/casm/pkg/boot/crawl"
	"github.com/wetware/casm/pkg/boot/survey"
)

func emit() *cli.Command {
	return &cli.Command{
		Name:  "emit",
		Usage: "emit a discovery packet",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "generate",
				Aliases: []string{"g"},
				Usage:   "generate test payload",
			},
			&cli.BoolFlag{
				Name:    "response",
				Aliases: []string{"resp", "res"},
				Usage:   "generate a response packet",
			},
			&cli.Uint64Flag{
				Name:    "distance",
				Aliases: []string{"dist"},
				Usage:   "generate a survey request with `dist`",
			},
			&cli.IntFlag{
				Name:    "repeat",
				Aliases: []string{"r"},
				Usage:   "send the same message `N` times",
				Value:   1,
			},
			&cli.DurationFlag{
				Name:    "interval",
				Aliases: []string{"i"},
				Usage:   "wait `duration` between sends",
				Value:   time.Second,
			},
		},
		Before: connectsock(),
		Action: send,
	}
}

func connectsock() cli.BeforeFunc {
	return func(c *cli.Context) (err error) {
		switch proto() {
		case crawl.P_CIDR:
			return dialunicast(c)

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

func dialunicast(c *cli.Context) error {
	network, address, err := manet.DialArgs(maddr)
	if err != nil {
		return err
	}

	addr, err = net.ResolveUDPAddr(network, address)
	if err != nil {
		return err
	}

	conn, err := net.ListenUDP(network, &net.UDPAddr{
		IP:   net.IPv4zero,
		Port: 0,
	})
	if err != nil {
		return err
	}

	return setsock(c, conn)
}

func send(c *cli.Context) error {
	e, err := payload(c)
	if err != nil {
		return err
	}

	for i := 0; repeat(c, i); i++ {
		if err = sock.Send(c.Context, e, addr); err != nil {
			break
		}

		select {
		case <-c.Done():
		case <-time.After(c.Duration("interval")):
		}
	}

	return err
}

func payload(c *cli.Context) (*record.Envelope, error) {
	if c.Bool("generate") {
		return generate(c)
	}

	// Read signed envelope at the command line
	b, err := ioutil.ReadAll(c.App.Reader)
	if err != nil {
		return nil, err
	}

	return record.UnmarshalEnvelope(b)
}

func repeat(c *cli.Context, i int) bool {
	if limit := c.Int("repeat"); limit > 0 {
		return i < limit
	}

	return true
}
