package routing

import (
	"encoding/binary"
	"math/rand"
	"testing"
	"time"

	"capnproto.org/go/capnp/v3"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multihash"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	t0 = time.Date(2020, 4, 9, 8, 0, 0, 0, time.UTC)
	id = newPeerID()
)

func TestIDIndexer(t *testing.T) {
	t.Parallel()
	t.Helper()

	t.Run("FromObject", func(t *testing.T) {
		t.Helper()

		t.Run("Record", func(t *testing.T) {
			rec := testRecord{id: id}
			ok, index, err := idIndexer{}.FromObject(rec)
			assert.NoError(t, err, "should index record")
			assert.True(t, ok, "record should have primary key")

			want := []byte(id)[2:]
			assert.Equal(t, want, index, "index should match 0x%x", want)
		})

		t.Run("ErrInvalidType", func(t *testing.T) {
			ok, index, err := idIndexer{}.FromObject("fail")
			assert.EqualError(t, err, "invalid type: string")
			assert.Nil(t, index, "should not return index")
			assert.False(t, ok, "should not return index")
		})
	})

	t.Run("FromArgs", func(t *testing.T) {
		t.Helper()

		t.Run("Succeed", func(t *testing.T) {
			t.Helper()

			for _, tt := range []struct {
				name string
				arg  any
			}{
				{name: "PeerID", arg: id},
				{name: "Base58", arg: id.String()},
				{name: "Bytes", arg: mustHashDigest([]byte(id))},
				{name: "Record", arg: testRecord{id: id}},
				{name: "Index", arg: testIndex{id: id}},
			} {
				t.Run(tt.name, func(t *testing.T) {
					index, err := idIndexer{}.FromArgs(tt.arg)
					assert.NoError(t, err, "should parse argument")

					want := []byte(id)[2:]
					assert.Equal(t, want, index, "index should match 0x%x", want)
				})
			}
		})

		t.Run("Fail", func(t *testing.T) {
			for _, tt := range []struct {
				name, emsg string
				args       []any
			}{
				{
					name: "ErrNumArgs",
					emsg: "expected one argument (got 2)",
					args: []any{"foo", "bar"},
				},
				{
					name: "ErrInvalidType",
					emsg: "invalid type: int",
					args: []any{42},
				},
				{
					name: "StringTooShort",
					emsg: multihash.ErrTooShort.Error(),
					args: []any{peer.ID("")},
				},
			} {
				t.Run(tt.name, func(t *testing.T) {
					index, err := idIndexer{}.FromArgs(tt.args...)
					assert.EqualError(t, err, tt.emsg)
					assert.Nil(t, index, "should not return index")
				})
			}
		})
	})
}

func BenchmarkIDIndexer(b *testing.B) {
	b.ReportAllocs()

	b.Run("FromObject", func(b *testing.B) {
		rec := testRecord{id: id}

		for i := 0; i < b.N; i++ {
			_, _, _ = idIndexer{}.FromObject(rec)
		}
	})

	b.Run("FromArgs", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = idIndexer{}.FromArgs(id)
		}
	})
}

func TestTimeIdexer(t *testing.T) {
	t.Parallel()
	t.Helper()

	ttl := time.Millisecond * 1024
	deadline := t0.Add(ttl)

	t.Run("FromObject", func(t *testing.T) {
		_, _, err := timeIndexer{}.FromObject(testRecord{})
		require.Error(t, err, "should fail if object is not *record")

		rec := &record{
			Record:   testRecord{},
			Deadline: deadline,
		}

		ok, index, err := timeIndexer{}.FromObject(rec)
		assert.NoError(t, err, "should index record")
		assert.True(t, ok, "record should have TTL index")

		ms := int64(binary.BigEndian.Uint64(index))
		assert.Equal(t, t0.Add(ttl).UnixNano(), ms,
			"index should be big-endian uint64 representing nanoseconds")
	})

	t.Run("FromArgs", func(t *testing.T) {
		index, err := timeIndexer{}.FromArgs(t0)
		assert.NoError(t, err, "should parse argument")

		want := make([]byte, 8)
		binary.BigEndian.PutUint64(want, uint64(t0.UnixNano()))

		assert.Equal(t, want, index,
			"index should be big-endian uint64 representing nanoseconds")
	})

	t.Run("OrderIsPreserved", func(t *testing.T) {
		ix0, err := timeIndexer{}.FromArgs(t0)
		require.NoError(t, err)

		ix1, err := timeIndexer{}.FromArgs(t0.Add(time.Millisecond))
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
	randsrc := rand.New(rand.NewSource(time.Now().UnixNano()))
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

type testIndex struct {
	id     peer.ID
	prefix bool
}

func (testIndex) String() string { return "id" }
func (t testIndex) Prefix() bool { return t.prefix }

func (t testIndex) PeerBytes() ([]byte, error) {
	return hashdigest([]byte(t.id))
}

func mustHashDigest(buf []byte) []byte {
	buf, err := hashdigest(buf)
	if err != nil {
		panic(err)
	}
	return buf
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
func (r testRecord) Instance() ID          { return ID(r.ins) }
func (r testRecord) Host() (string, error) { return r.host, nil }
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
