//go:generate mockgen -source=sealer.go -destination=../../internal/mock/pkg/socket/sealer.go -package=mock_socket

package socket

import (
	"capnproto.org/go/capnp/v3/exp/bufferpool"
	"github.com/cespare/xxhash/v2"
	lru "github.com/hashicorp/golang-lru/v2"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/record"
)

// Sealer can sign and marshal a record.  Implementations SHOULD
// cache results.
type Sealer interface {
	Seal(record.Record) ([]byte, error)
}

// Hashable is an optional interface that records MAY provide in
// order to override the default key-derivation function.   This
// may be desireable when MarshalRecord() allocates heavily. Use
// of xxhash is RECOMMENDED.
//
// See boot.Record for an example.
type Hashable interface {
	Hash() (uint64, error)
}

// CachingSealer uses a simple LRU-cache strategy to reduce CPU
// load due to repeated signing.   Additionally, it returns the
// signed bytes to a global free-list upon eviction.
type CachingSealer struct {
	pk    crypto.PrivKey
	cache *lru.Cache[uint64, []byte]
}

func NewCachingSealerFromHost(h host.Host, size int) (*CachingSealer, error) {
	return NewCachingSealer(h.Peerstore().PrivKey(h.ID()), size)
}

func NewCachingSealer(pk crypto.PrivKey, size int) (*CachingSealer, error) {
	cache, err := lru.NewWithEvict(size, func(_ uint64, b []byte) {
		bufferpool.Default.Put(b)
	})

	return &CachingSealer{
		pk:    pk,
		cache: cache,
	}, err
}

func (c *CachingSealer) Seal(r record.Record) ([]byte, error) {
	key, err := c.deriveKey(r)
	if err != nil {
		return nil, err
	}

	b, ok := c.cache.Get(key)
	if ok {
		return b, nil
	}

	e, err := record.Seal(r, c.pk)
	if err != nil {
		return nil, err
	}

	if b, err = e.Marshal(); err == nil {
		c.cache.Add(key, b)
	}

	return b, err
}

func (c *CachingSealer) Purge() {
	c.cache.Purge()
}

func (c *CachingSealer) deriveKey(r record.Record) (uint64, error) {
	if h, ok := r.(Hashable); ok {
		return h.Hash()
	}

	b, err := r.MarshalRecord()
	defer bufferpool.Default.Put(b) // maybe something else will use it

	return xxhash.Sum64(b), err
}
