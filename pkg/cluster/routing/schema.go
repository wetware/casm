package routing

import (
	"encoding/hex"
	"fmt"
	"reflect"
	"time"

	"github.com/hashicorp/go-memdb"
	"github.com/libp2p/go-libp2p/core/peer"
	b58 "github.com/mr-tron/base58/base58"
	"go.uber.org/atomic"
)

func schema(clock *atomic.Time) *memdb.TableSchema {
	return &memdb.TableSchema{
		Name: "record",
		Indexes: map[string]*memdb.IndexSchema{
			"id": {
				Name:    "id",
				Unique:  true,
				Indexer: peerIndexer{},
			},
			"ttl": {
				Name:    "ttl",
				Indexer: timeIndexer{},
			},
			"instance": {
				Name:    "instance",
				Indexer: instanceIndexer{},
			},
			"host": {
				Name:    "host",
				Indexer: hostnameIndexer{},
			},
			"meta": {
				Name:         "meta",
				AllowMissing: true,
				Indexer:      metaIndexer{},
			},
		},
	}
}

type peerIndexer struct{}

func (peerIndexer) FromObject(obj any) (bool, []byte, error) {
	switch rec := obj.(type) {
	case PeerIndex:
		index, err := rec.PeerBytes()
		return err == nil, index, err

	case Record:
		id := rec.Peer()
		return true, []byte(id), nil
	}

	return false, nil, errType(obj)
}

func (peerIndexer) FromArgs(args ...any) ([]byte, error) {
	if len(args) == 0 {
		return nil, errNArgs(args)
	}

	switch id := args[0].(type) {
	case PeerIndex:
		return id.PeerBytes()

	case Record:
		return []byte(id.Peer()), nil

	case string:
		return b58.Decode(id)

	case peer.ID:
		return []byte(id), nil
	}

	return nil, errType(args)
}

func (peerIndexer) PrefixFromArgs(args ...any) ([]byte, error) {
	return peerIndexer{}.FromArgs(args...)
}

type instanceIndexer struct{}

func (instanceIndexer) FromObject(obj any) (bool, []byte, error) {
	if rec, ok := obj.(Record); ok {
		return true, rec.Instance().Bytes(), nil
	}

	return false, nil, errType(obj)
}

func (instanceIndexer) FromArgs(args ...any) ([]byte, error) {
	if len(args) != 1 {
		return nil, errNArgs(args)
	}

	switch id := args[0].(type) {
	case ID:
		return id.Bytes(), nil

	case string:
		index := make([]byte, 4)
		_, err := hex.Decode(index, []byte(id))
		return index, err
	}

	return nil, errType(args)
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
		return true, index, err

	case Record:
		name, err := rec.Host()
		return true, []byte(name), err
	}

	return false, nil, errType(obj)
}

func (hostnameIndexer) FromArgs(args ...any) ([]byte, error) {
	name, err := argsToString(args...)
	return []byte(name), err
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
