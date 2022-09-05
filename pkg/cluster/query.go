package cluster

import (
	"fmt"

	"capnproto.org/go/capnp/v3"
	api "github.com/wetware/casm/internal/api/routing"
	"github.com/wetware/casm/pkg/cluster/routing"
)

func All(cs ...Constraint) Query {
	return NewQuery(func(s api.View_Selector) error {
		s.SetAll()
		return nil
	}, cs...)
}

func Select(index routing.Index, cs ...Constraint) Query {
	return NewQuery(func(s api.View_Selector) error {
		return bindIndex(s.NewMatch, index)
	}, cs...)
}

func From(index routing.Index, cs ...Constraint) Query {
	return NewQuery(func(s api.View_Selector) error {
		return bindIndex(s.NewFrom, index)
	}, cs...)
}

func bindIndex(fn func() (api.View_Index, error), index routing.Index) error {
	target, err := fn()
	if err != nil {
		return err
	}

	switch index.Key() {
	case routing.PeerKey:
		return bindPeer(target, index)

	case routing.PeerPrefixKey:
		return bindPeerPrefix(target, index)

	case routing.HostKey:
		return bindHost(target, index)

	case routing.HostPrefixKey:
		return bindHostPrefix(target, index)

	case routing.MetaKey:
		return bindMeta(target, index)

	case routing.MetaPrefixKey:
		return bindMetaPrefix(target, index)
	}

	return fmt.Errorf("invalid index: %s", index)
}

func bindPeer(ix api.View_Index, index routing.Index) error {
	p, err := index.(interface{ Peer() (string, error) }).Peer()
	if err == nil {
		err = ix.SetPeer(p)
	}

	return err
}

func bindPeerPrefix(ix api.View_Index, index routing.Index) error {
	p, err := index.(interface{ PeerPrefix() (string, error) }).PeerPrefix()
	if err == nil {
		err = ix.SetPeer(p)
	}

	return err
}

func bindHost(ix api.View_Index, index routing.Index) error {
	h, err := index.(interface{ Host() (string, error) }).Host()
	if err == nil {
		err = ix.SetHost(h)
	}

	return err
}

func bindHostPrefix(ix api.View_Index, index routing.Index) error {
	h, err := index.(interface{ HostPrefix() (string, error) }).HostPrefix()
	if err == nil {
		err = ix.SetHost(h)
	}

	return err
}

func bindMeta(ix api.View_Index, index routing.Index) error {
	m, err := index.(interface {
		Meta() (capnp.TextList, error)
	}).Meta()
	if err == nil {
		err = ix.SetMeta(m)
	}

	return err
}

func bindMetaPrefix(ix api.View_Index, index routing.Index) error {
	m, err := index.(interface {
		MetaPrefix() (capnp.TextList, error)
	}).MetaPrefix()
	if err == nil {
		err = ix.SetMeta(m)
	}

	return err
}
