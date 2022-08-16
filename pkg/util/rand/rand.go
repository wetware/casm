package randutil

import (
	"math/rand"
	"unsafe"

	"github.com/libp2p/go-libp2p-core/peer"
)

// SourceFromID returns a random source that is seeded with the last
// 8 bytes of the supplied peer.ID.  This provides randomness across
// hosts, while making samplings reproducible within a given host.
//
// This is most often used in the context of random jitters, where
// it presents the advantage of facilitating the analysis of cluster
// logs.  Note that this MUST NOT be used in cases where adversaries
// cannot be allowed to guess the random sequence.
func SourceFromID(id peer.ID) rand.Source {
	if len(id) < 8 {
		panic("invalid peer.ID (length must be >= 8)")
	}

	b := *(*[]byte)(unsafe.Pointer(&id))
	b = b[len(b)-8:]

	return rand.NewSource(*(*int64)(unsafe.Pointer(&b)))
}
