package routing

import (
	"math/rand"
	"testing"

	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/assert"
)

func TestStringCastUnsafe(t *testing.T) {
	t.Parallel()

	id := newPeerID()

	for _, tt := range []struct {
		name, want string
		v          interface{}
	}{
		{
			name: "peer.ID",
			want: string(id),
			v:    id,
		},
		{
			name: "string",
			want: "test",
			v:    "test",
		},
	} {
		assert.Equal(t, tt.want, stringCastUnsafe(tt.v))
	}
}

func TestPeerIDCastUnsafe(t *testing.T) {
	t.Parallel()

	id := newPeerID()
	assert.Equal(t, id, idCastUnsafe(string(id)))
}

func newPeerID() peer.ID {
	// use non-cryptographic source; it's just a test.
	sk, _, err := crypto.GenerateECDSAKeyPair(rand.New(rand.NewSource(42)))
	if err != nil {
		panic(err)
	}

	id, err := peer.IDFromPrivateKey(sk)
	if err != nil {
		panic(err)
	}

	return id
}
