//go:generate mockgen -source=routing.go -destination=../../../internal/mock/pkg/cluster/routing/routing.go -package=mock_routing

package routing

import (
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"capnproto.org/go/capnp/v3"
	"github.com/libp2p/go-libp2p/core/peer"
)

// ID is an opaque identifier that identifies a unique host instance
// on the network.   A fresh ID is generated for each cluster.Router
// instance, making it possible to distinguish between multiple runs
// of a libp2p host with a fixed peer identity.
//
// IDs are not guaranteed to be globally unique.
type ID uint32

func (id ID) String() string {
	return hex.EncodeToString(id.Bytes())
}

func (id ID) MarshalText() ([]byte, error) {
	return []byte(id.String()), nil
}

func (id ID) Bytes() []byte {
	return []byte{ // little-endian encoding
		byte(id),
		byte(id >> 8),
		byte(id >> 16),
		byte(id >> 24)}
}

// Record is an entry in the routing table.
type Record interface {
	Peer() peer.ID
	TTL() time.Duration
	Seq() uint64
	Instance() ID
	Host() (string, error)
	Meta() (Meta, error)
}

// Snapshot provides iteration strategies over an isolated snapshot
// of the routing-table.  Implementations MUST NOT mutate the state
// of the routing table, and MUST support concurrent iteration.
type Snapshot interface {
	Get(Index) (Iterator, error)
	GetReverse(Index) (Iterator, error)
	LowerBound(Index) (Iterator, error)
	ReverseLowerBound(Index) (Iterator, error)
}

// Index is a pointer to a 'column' in the routing table's schema.
// Indexes MUST implement index methods corresponding to the index
// name returned by String().  See schema.go for more information.
type Index interface {
	// String returns the index name.
	String() string

	// Prefix returns true if the index is a prefix match
	Prefix() bool
}

// PeerIndex is an optional interface that Records may implement
// to provide fast, allocation-free construction of peer indexes.
type PeerIndex interface {
	PeerBytes() ([]byte, error)
}

// HostIndex is an optional interface that Records may implement
// to provide fast, allocation-free construction of host indexes.
type HostIndex interface {
	HostBytes() ([]byte, error)
}

// Iterator is a stateful object that enumerates routing
// records.  Iterator's methods are NOT guaranteed to be
// thread-safe, but implementations MUST permit multiple
// iterators to operate concurently.
//
// Implementations MAY operate on immutable snapshots of
// routing-table state, so callers SHOULD consume record
// streams promptly.
type Iterator interface {
	// Next pops a record from the head of the stream and
	// returns it to the caller. Subsequent calls to Next
	// will return a different record. When the stream is
	// exhausted, Next returns nil.
	Next() Record
}

// Meta is an indexed set of key-value pairs describing
// arbitrary metadata.
type Meta capnp.TextList

func (m Meta) String() string {
	return capnp.TextList(m).String()
}

// Len returns the number of metadata fields present in
// the set.
func (m Meta) Len() int {
	return capnp.TextList(m).Len()
}

// At returns the metadata field at index i.
func (m Meta) At(i int) (Field, error) {
	s, err := capnp.TextList(m).At(i)
	if err != nil {
		return Field{}, err
	}

	return ParseField(s)
}

// Get returns the value associated with the supplied key.
// If the key is not found, Get returns ("", nil).  Errors
// are reserved for failures in reading or parsing fields.
func (m Meta) Get(key string) (string, error) {
	for i := 0; i < m.Len(); i++ {
		field, err := m.At(i)
		if err != nil {
			return "", err
		}

		if field.Key == key {
			return field.Value, err
		}
	}

	return "", nil
}

// Index returns a set of indexes for the metadata fields.
func (m Meta) Index() (indexes [][]byte, err error) {
	var index []byte
	for i := 0; i < m.Len(); i++ {
		index, err = capnp.TextList(m).BytesAt(i)
		if err != nil {
			break
		}

		indexes = append(indexes, index)
	}

	return
}

// Field is a key-value pair.
type Field struct {
	Key, Value string
}

func ParseField(s string) (Field, error) {
	switch ss := strings.Split(s, "="); len(ss) {
	case 0:
		return Field{}, errors.New("missing key")

	case 1:
		return Field{}, errors.New("separator not found")

	default:
		return Field{
			Key:   ss[0],
			Value: strings.Join(ss[1:], "="),
		}, nil

	}
}

func (f Field) String() (s string) {
	if f.Key != "" {
		s = f.Key + "=" + f.Value
	}

	return
}
