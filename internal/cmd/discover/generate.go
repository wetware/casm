package discover

import (
	"bytes"
	"crypto/rand"
	"io"

	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/record"
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

	e, err := record.Seal(rec, pk)
	if err != nil {
		return nil, err
	}

	if err = cache.Reset(e); err != nil {
		return nil, err
	}

	return cache.LoadRequest(pk, c.String("ns"))
}

func privkey() (crypto.PrivKey, error) {
	pk, _, err := crypto.GenerateECDSAKeyPair(rand.Reader)
	return pk, err
}
