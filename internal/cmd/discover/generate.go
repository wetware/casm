package discover

import (
	"bytes"
	"io"

	"github.com/mikelsr/go-libp2p"
	inproc "github.com/mikelsr/go-libp2p-inproc-transport"
	"github.com/mikelsr/go-libp2p/core/host"
	"github.com/mikelsr/go-libp2p/core/record"
	"github.com/urfave/cli/v2"
	"github.com/wetware/casm/pkg/boot/socket"
)

func genpayload() *cli.Command {
	return &cli.Command{
		Name:  "genpayload",
		Usage: "generate signed payload for testing",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "response",
				Aliases: []string{"res", "resp"},
				Usage:   "generate a response packet",
			},
			&cli.Uint64Flag{
				Name:    "distance",
				Aliases: []string{"dist"},
				Usage:   "generate a survey request with `dist`",
			},
		},
		Action: func(c *cli.Context) error {
			e, err := generate(c)
			if err != nil {
				return err
			}

			b, err := e.Marshal()
			if err != nil {
				return err
			}

			_, err = io.Copy(c.App.Writer, bytes.NewReader(b))
			return err
		},
	}
}

func generate(c *cli.Context) (*record.Envelope, error) {
	cache := socket.NewCache(8)

	h, err := libp2p.New(
		libp2p.NoListenAddrs,
		libp2p.NoTransports,
		libp2p.Transport(inproc.New()),
		libp2p.ListenAddrStrings("/inproc/~"))
	if err != nil {
		return nil, err
	}

	seal := sealer(h)

	if c.Bool("response") {
		return cache.LoadResponse(seal, h, c.String("ns"))
	}

	if c.IsSet("distance") {
		return cache.LoadSurveyRequest(seal,
			h.ID(),
			c.String("ns"),
			uint8(c.Uint("distance")))
	}

	return cache.LoadRequest(seal, h.ID(), c.String("ns"))
}

func sealer(h host.Host) func(record.Record) (*record.Envelope, error) {
	return func(r record.Record) (*record.Envelope, error) {
		return record.Seal(r, h.Peerstore().PrivKey(h.ID()))
	}
}
