package discover

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"

	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/record"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/urfave/cli/v2"
	"github.com/wetware/casm/pkg/boot/socket"
)

var (
	pk    crypto.PrivKey
	id    peer.ID
	cache = socket.NewRecordCache(8)
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
	pk, err := privkey()
	if err != nil {
		return nil, err
	}

	rec := peer.NewPeerRecord()
	rec.PeerID, err = peer.IDFromPrivateKey(pk)
	if err != nil {
		return nil, err
	}

	rec.Addrs = append(rec.Addrs, ma.StringCast(fmt.Sprintf(
		"/ip4/10.0.0.1/udp/2020/quic/p2p/%s", rec.PeerID)))
	rec.Addrs = append(rec.Addrs, ma.StringCast(fmt.Sprintf(
		"/ip6/::1/udp/2020/quic/p2p/%s", rec.PeerID)))

	e, err := record.Seal(rec, pk)
	if err != nil {
		return nil, err
	}

	if err = cache.Reset(e); err != nil {
		return nil, err
	}

	if c.Bool("resp") {
		return cache.LoadResponse(pk, c.String("ns"))
	}

	if c.IsSet("dist") {
		return cache.LoadGradualRequest(pk,
			c.String("ns"),
			uint8(c.Uint("dist")))
	}

	return cache.LoadRequest(pk, c.String("ns"))
}

func privkey() (crypto.PrivKey, error) {
	pk, _, err := crypto.GenerateECDSAKeyPair(rand.Reader)
	return pk, err
}
