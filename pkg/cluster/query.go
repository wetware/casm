package cluster

import (
	"errors"
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

	target.SetPrefix(index.Prefix())

	switch index.String() {
	case "id":
		return bindPeer(target, index)

	case "host":
		return bindHost(target, index)

	case "meta":
		return bindMeta(target, index)
	}

	return fmt.Errorf("invalid index: %s", index)
}

func bindPeer(target api.View_Index, index routing.Index) error {
	switch ix := index.(type) {
	case routing.PeerIndex:
		b, err := ix.PeerBytes()
		if err == nil {
			return target.SetId(string(b)) // TODO:  unsafe.Pointer
		}
		return err

	case interface{ Peer() (string, error) }:
		id, err := ix.Peer()
		if err == nil {
			err = target.SetId(id)
		}
		return err
	}

	return errors.New("not a peer index")
}

func bindHost(target api.View_Index, index routing.Index) error {
	switch ix := index.(type) {
	case routing.HostIndex:
		b, err := ix.HostBytes()
		if err == nil {
			return target.SetHost(string(b)) // TODO:  unsafe.Pointer
		}
		return err

	case interface{ Host() (string, error) }:
		id, err := ix.Host()
		if err == nil {
			err = target.SetHost(id)
		}
		return err
	}

	return errors.New("not a peer index")
}

func bindMeta(target api.View_Index, index routing.Index) error {
	m, err := index.(interface{ Meta() (routing.Meta, error) }).Meta()
	if err == nil {
		err = target.SetMeta(capnp.TextList(m))
	}

	return err
}
