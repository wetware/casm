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
	pk crypto.PrivKey
	id peer.ID
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
	cache, err := socket.NewRecordCache(8)
	if err != nil {
		return nil, err
	}

	pk, err := privkey()
	if err != nil {
		return nil, err
	}

	rec, err := newPeerRecord(pk)
	if err != nil {
		return nil, err
	}

	seal := sealer(pk)

	e, err := seal(rec)
	if err != nil {
		return nil, err
	}

	if err = cache.Reset(e); err != nil {
		return nil, err
	}

	if c.Bool("resp") {
		return cache.LoadResponse(seal, c.String("ns"))
	}

	if c.IsSet("dist") {
		return cache.LoadSurveyRequest(seal,
			rec.PeerID,
			c.String("ns"),
			uint8(c.Uint("dist")))
	}

	return cache.LoadRequest(seal, rec.PeerID, c.String("ns"))
}

func newPeerRecord(pk crypto.PrivKey) (rec *peer.PeerRecord, err error) {
	rec = peer.NewPeerRecord()
	rec.PeerID, err = peer.IDFromPrivateKey(pk)
	if err != nil {
		return nil, err
	}

	rec.Addrs = append(rec.Addrs, ma.StringCast(fmt.Sprintf(
		"/ip4/10.0.0.1/udp/2020/quic/p2p/%s", rec.PeerID)))
	rec.Addrs = append(rec.Addrs, ma.StringCast(fmt.Sprintf(
		"/ip6/::1/udp/2020/quic/p2p/%s", rec.PeerID)))

	return
}

func sealer(pk crypto.PrivKey) func(record.Record) (*record.Envelope, error) {
	return func(r record.Record) (*record.Envelope, error) {
		return record.Seal(r, pk)
	}
}

func privkey() (crypto.PrivKey, error) {
	pk, _, err := crypto.GenerateECDSAKeyPair(rand.Reader)
	return pk, err
}
