package routing

import (
	"errors"
	"strings"
	"time"

	"capnproto.org/go/capnp/v3"
	"github.com/libp2p/go-libp2p-core/peer"
)

type Record interface {
	Peer() peer.ID
	TTL() time.Duration
	Seq() uint64
	Instance() uint32
	Host() (string, error)
	Meta() (Meta, error)
}

type Query interface {
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

	// Match returns true if the index matches the supplied record.
	Match(Record) bool
}

/*
 * Optional index interfaces.
 */

type PeerIndex interface {
	PeerBytes() ([]byte, error)
}

type HostIndex interface {
	HostBytes() ([]byte, error)
}

type MetaIndex interface {
	MetaBytes() ([][]byte, error)
}

// Iterator is a stateful object that enumerates routing
// records.  Iterator's methods are NOT guaranteed to be
// thread-safe, but implementations MUST permit multiple
// iterators to operate concurently.
//
// Iterators are snapshots of the routing table and SHOULD
// be consumed quickly.
type Iterator interface {
	// Next updates the iterator's internal state, causing
	// Deadline() to return a different value when called.
	// If Next() returns nil, the iterator is exhausted.
	//
	// The returned record is valid until the next call to
	// Next().
	Next() Record
}

type Meta capnp.TextList

func (m Meta) Len() int {
	return capnp.TextList(m).Len()
}

func (m Meta) At(i int) (Field, error) {
	s, err := capnp.TextList(m).At(i)
	if err != nil {
		return Field{}, err
	}

	return parseField(s)
}

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

func (m Meta) Index() (indexes [][]byte, err error) {
	indexes = make([][]byte, m.Len())
	for i := range indexes {
		if indexes[i], err = capnp.TextList(m).BytesAt(i); err != nil {
			break
		}
	}

	return
}

type Field struct {
	Key, Value string
}

func parseField(s string) (Field, error) {
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
