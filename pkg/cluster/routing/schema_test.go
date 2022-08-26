package routing

import (
	"encoding/binary"
	"math/rand"
	"testing"
	"time"

	"capnproto.org/go/capnp/v3"
	"go.uber.org/atomic"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	t0      = time.Date(2020, 4, 9, 8, 0, 0, 0, time.UTC)
	randsrc = rand.New(rand.NewSource(time.Now().UnixNano()))
)

func TestPeerIndex(t *testing.T) {
	t.Parallel()
	t.Helper()

	id := newPeerID()

	t.Run("FromObject", func(t *testing.T) {
		rec := testRecord{id: id}
		ok, index, err := peerIndexer{}.FromObject(rec)
		assert.NoError(t, err, "should index record")
		assert.Equal(t, id, peer.ID(index), "index should match peer.ID")
		assert.True(t, ok, "record should have peer index")
	})

	t.Run("FromArgs", func(t *testing.T) {
		t.Run("PeerID", func(t *testing.T) {
			index, err := peerIndexer{}.FromArgs(id)
			assert.NoError(t, err, "should parse argument")
			assert.Equal(t, id, peer.ID(index), "index should match peer.ID")
		})

		t.Run("String", func(t *testing.T) {
			index, err := peerIndexer{}.FromArgs(id.String())
			assert.NoError(t, err, "should parse argument")
			assert.Equal(t, id, peer.ID(index), "index should match peer.ID")
		})
	})

	t.Run("PrefixFromArgs", func(t *testing.T) {
		t.Run("PeerID", func(t *testing.T) {
			index, err := peerIndexer{}.PrefixFromArgs(id[:4])
			assert.NoError(t, err, "should parse argument")
			assert.Equal(t, id[:4], peer.ID(index), "index should match peer.ID")
		})

		t.Run("String", func(t *testing.T) {
			index, err := peerIndexer{}.PrefixFromArgs(id[:4].String())
			assert.NoError(t, err, "should parse argument")
			assert.Equal(t, id[:4], peer.ID(index), "index should match peer.ID")
		})
	})
}

func TestInstanceIndexer(t *testing.T) {
	t.Parallel()
	t.Helper()

	var i uint32 = 42

	t.Run("FromObject", func(t *testing.T) {
		rec := testRecord{ins: i}
		ok, index, err := instanceIndexer().FromObject(rec)
		assert.NoError(t, err, "should index record")
		assert.True(t, ok, "record should have peer index")
		assert.Equal(t, i,
			binary.LittleEndian.Uint32(index), "index should be %d", i)
	})

	t.Run("FromArgs", func(t *testing.T) {
		index, err := instanceIndexer().FromArgs(i)
		assert.NoError(t, err,
			"should parse argument")
		assert.Equal(t, i, binary.LittleEndian.Uint32(index),
			"index should be little-endian encoding of %d", 42)
	})
}

func TestTimeIdexer(t *testing.T) {
	t.Parallel()
	t.Helper()

	ttl := time.Millisecond * 1024
	clock := atomic.NewTime(t0)

	t.Run("FromObject", func(t *testing.T) {
		rec := testRecord{ttl: ttl}
		ok, index, err := (*timeIndexer)(clock).FromObject(rec)
		assert.NoError(t, err, "should index record")
		assert.True(t, ok, "record should have TTL index")

		ms := int64(binary.BigEndian.Uint64(index))
		assert.Equal(t, t0.Add(ttl).UnixNano(), ms,
			"index should be big-endian uint64 representing nanoseconds")
	})

	t.Run("FromArgs", func(t *testing.T) {
		index, err := (*timeIndexer)(clock).FromArgs(t0)
		assert.NoError(t, err, "should parse argument")

		want := make([]byte, 8)
		binary.BigEndian.PutUint64(want, uint64(t0.UnixNano()))

		assert.Equal(t, want, index,
			"index should be big-endian uint64 representing nanoseconds")
	})

	t.Run("OrderIsPreserved", func(t *testing.T) {
		ix0, err := (*timeIndexer)(clock).FromArgs(t0)
		require.NoError(t, err)

		ix1, err := (*timeIndexer)(clock).FromArgs(t0.Add(time.Millisecond))
		require.NoError(t, err)

		require.Less(t, ix0, ix1, "should preserve time ordering (ix0 < ix1)")
	})
}

func TestHostIndexer(t *testing.T) {
	t.Parallel()
	t.Helper()

	const name = "foobar"

	t.Run("FromObject", func(t *testing.T) {
		rec := testRecord{host: name}
		ok, index, err := hostnameIndexer{}.FromObject(rec)
		assert.NoError(t, err, "should index record")
		assert.Equal(t, name, string(index), "index should match hostname")
		assert.True(t, ok, "record should have peer index")
	})

	t.Run("FromArgs", func(t *testing.T) {
		index, err := hostnameIndexer{}.FromArgs(name)
		assert.NoError(t, err, "should parse argument")
		assert.Equal(t, name, string(index), "index should match hostname")
	})

	t.Run("PrefixFromArgs", func(t *testing.T) {
		const prefix = "foo"
		index, err := hostnameIndexer{}.PrefixFromArgs(prefix)
		assert.NoError(t, err, "should parse prefix argument")
		assert.Equal(t, prefix, string(index), "index should match prefix")
	})
}

func TestMetaIndexer(t *testing.T) {
	t.Parallel()
	t.Helper()

	meta := newMeta(
		"key1=value1",
		"key2=value2")

	t.Run("FromObject", func(t *testing.T) {
		rec := testRecord{meta: meta}
		ok, indexes, err := metaIndexer{}.FromObject(rec)
		assert.NoError(t, err, "should index record")
		assert.True(t, ok, "record should have meta indexes")
		assert.Len(t, indexes, meta.Len(),
			"should index %d key-value pairs", meta.Len())
	})

	t.Run("FromArgs", func(t *testing.T) {
		index, err := metaIndexer{}.FromArgs("key1=value1")
		assert.NoError(t, err, "should parse key into value")
		assert.Equal(t, "key1=value1", string(index),
			"index should be key-value pair")
	})

	t.Run("PrefixFromArgs", func(t *testing.T) {
		index, err := metaIndexer{}.PrefixFromArgs("key1=val")
		assert.NoError(t, err, "should parse key into value")
		assert.Equal(t, "key1=val", string(index),
			"index should be key-value pair")
	})
}

func newPeerID() peer.ID {
	sk, _, err := crypto.GenerateEd25519Key(randsrc)
	if err != nil {
		panic(err)
	}

	id, err := peer.IDFromPrivateKey(sk)
	if err != nil {
		panic(err)
	}

	return id
}

type testRecord struct {
	id   peer.ID
	seq  uint64
	ins  uint32
	host string
	meta Meta
	ttl  time.Duration
}

func (r testRecord) Peer() peer.ID         { return r.id }
func (r testRecord) Seq() uint64           { return r.seq }
func (r testRecord) Host() (string, error) { return r.host, nil }
func (r testRecord) Instance() uint32      { return r.ins }
func (r testRecord) Meta() (Meta, error)   { return r.meta, nil }
func (r testRecord) TTL() time.Duration {
	if r.ttl == 0 {
		return time.Second
	}

	return r.ttl
}

func newMeta(ss ...string) Meta {
	_, seg := capnp.NewSingleSegmentMessage(nil)
	meta, _ := capnp.NewTextList(seg, int32(len(ss)))
	for i, s := range ss {
		meta.Set(i, s)
	}
	return Meta(meta)
}
