package routing

import (
	"fmt"
	"reflect"
	"time"
	"unsafe"

	"github.com/hashicorp/go-memdb"
	"github.com/libp2p/go-libp2p/core/peer"
	b58 "github.com/mr-tron/base58/base58"
	"github.com/multiformats/go-multihash"
	"github.com/multiformats/go-varint"
	"go.uber.org/atomic"
)

func schema(clock *atomic.Time) *memdb.TableSchema {
	return &memdb.TableSchema{
		Name: "record",
		Indexes: map[string]*memdb.IndexSchema{
			"id": {
				Name:    "id",
				Unique:  true,
				Indexer: idIndexer{},
			},
			"ttl": {
				Name:    "ttl",
				Indexer: timeIndexer{},
			},
			"host": {
				Name:         "host",
				AllowMissing: true,
				Indexer:      hostnameIndexer{},
			},
			"meta": {
				Name:         "meta",
				AllowMissing: true,
				Indexer:      metaIndexer{},
			},
		},
	}
}

type idIndexer struct{}

func (idIndexer) FromObject(obj any) (bool, []byte, error) {
	switch r := obj.(type) {
	case PeerIndex:
		peerID, err := r.PeerBytes()
		if err == nil {
			peerID, err = hashdigest(peerID)
		}
		return err == nil, peerID, err

	case Record:
		peer := r.Peer()
		index, err := hashdigest(*(*[]byte)(unsafe.Pointer(&peer)))
		return err == nil, index, err
	}

	return false, nil, errType(obj)
}

func (idIndexer) FromArgs(args ...any) ([]byte, error) {
	if len(args) != 1 {
		return nil, errNArgs(args)
	}

	switch arg := args[0].(type) {
	case Index:
		return arg.(PeerIndex).PeerBytes()

	case []byte:
		return arg, nil

	case peer.ID:
		id := arg // required for unsafe.Pointer to be have correctly
		return hashdigest(*(*[]byte)(unsafe.Pointer(&id)))

	case string:
		index, err := b58.Decode(arg)
		if err == nil {
			index, err = hashdigest(index)
		}
		return index, err

	case Record:
		_, index, err := idIndexer{}.FromObject(arg)
		return index, err
	}

	return nil, errType(args[0])
}

func (idIndexer) PrefixFromArgs(args ...any) ([]byte, error) {
	return idIndexer{}.FromArgs(args...)
}

// hashdigest trims the headers from a multihash and returns the
// hash digest.
func hashdigest(b []byte) (_ []byte, err error) {
	if len(b) < 2 {
		return nil, multihash.ErrTooShort
	}

	// read the hash code, followed by the length header
	// https://github.com/multiformats/multihash#format
	var n int
	for i := 0; i < 2; i++ {
		if _, n, err = varint.FromUvarint(b); err == nil {
			b = b[n:]
		}
	}

	return b, err
}

type timeIndexer struct{}

func (timeIndexer) FromObject(obj any) (bool, []byte, error) {
	if r, ok := obj.(*record); ok {
		return true, timeToBytes(r.Deadline), nil
	}

	return false, nil, errType(obj)
}

func (timeIndexer) FromArgs(args ...any) ([]byte, error) {
	t, err := argsToTime(args...)
	if err != nil {
		return nil, err
	}

	return timeToBytes(t), nil
}

func timeToBytes(t time.Time) []byte {
	ms := t.UnixNano()
	return []byte{
		byte(ms >> 56),
		byte(ms >> 48),
		byte(ms >> 40),
		byte(ms >> 32),
		byte(ms >> 24),
		byte(ms >> 16),
		byte(ms >> 8),
		byte(ms)}
}

func argsToTime(args ...any) (time.Time, error) {
	if len(args) != 1 {
		return time.Time{}, errNArgs(args)
	}

	if t, ok := args[0].(time.Time); ok {
		return t, nil
	}

	return time.Time{}, errType(args[0])
}

type hostnameIndexer struct{}

func (hostnameIndexer) FromObject(obj any) (bool, []byte, error) {
	switch rec := obj.(type) {
	case HostIndex:
		index, err := rec.HostBytes()
		return len(index) != 0, index, err

	case Record:
		name, err := rec.Host()
		if name == "" {
			return false, nil, err
		}

		return true, []byte(name), err // TODO:  unsafe.Pointer ?
	}

	return false, nil, errType(obj)
}

func (hostnameIndexer) FromArgs(args ...any) ([]byte, error) {
	name, err := argsToString(args...)
	return []byte(name), err // TODO:  unsafe.Pointer ?
}

func (hostnameIndexer) PrefixFromArgs(args ...any) ([]byte, error) {
	return hostnameIndexer{}.FromArgs(args...)
}

func argsToString(args ...any) (string, error) {
	if len(args) != 1 {
		return "", errNArgs(args)
	}

	if s, ok := args[0].(string); ok {
		return s, nil
	}

	return "", errNArgs(args)
}

type metaIndexer struct{}

func (metaIndexer) FromObject(obj any) (bool, [][]byte, error) {
	if r, ok := obj.(Record); ok {
		meta, err := r.Meta()
		if err != nil || meta.Len() == 0 {
			return false, nil, err
		}

		indexes, err := meta.Index()
		return true, indexes, err
	}

	return false, nil, errType(obj)
}

func (metaIndexer) FromArgs(args ...any) ([]byte, error) {
	key, err := argsToString(args...)
	return []byte(key), err
}

func (metaIndexer) PrefixFromArgs(args ...any) ([]byte, error) {
	return metaIndexer{}.FromArgs(args...)
}

func errType(v any) error {
	return fmt.Errorf("invalid type: %s", reflect.TypeOf(v))
}

func errNArgs(args []any) error {
	return fmt.Errorf("expected one argument (got %d)", len(args))
}
