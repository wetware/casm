package view

import (
	"context"
	"fmt"

	"capnproto.org/go/capnp/v3"

	api "github.com/wetware/casm/internal/api/routing"
	"github.com/wetware/casm/pkg/cluster/pulse"
	"github.com/wetware/casm/pkg/cluster/query"
	"github.com/wetware/casm/pkg/cluster/routing"
	streamutil "github.com/wetware/casm/pkg/util/stream"
)

type RoutingTable interface {
	Snapshot() routing.Snapshot
}

type RecordBinder interface {
	BindRecord(api.View_Record) error
}

type Server struct {
	RoutingTable
}

func (s Server) View() View {
	return View(api.View_ServerToClient(s))
}

func (s Server) Lookup(ctx context.Context, call api.View_lookup) error {
	sel, err := call.Args().Selector()
	if err != nil {
		return err
	}

	return s.bind(maybeRecord(call), selector(sel).Bind(query.First()))
}

func (s Server) Iter(ctx context.Context, call api.View_iter) error {
	sel, err := call.Args().Selector()
	if err != nil {
		return err
	}

	return s.bind(iterator(ctx, call), selector(sel))
}

func (s Server) Reverse(ctx context.Context, call api.View_reverse) error {
	return fmt.Errorf("NOT IMPLEMENTED") // XXX
}

func selector(s api.View_Selector) query.Selector {
	switch s.Which() {
	case api.View_Selector_Which_all:
		return query.All()

	case api.View_Selector_Which_match:
		match, err := s.Match()
		if err != nil {
			return query.Failure(err)
		}

		return query.Select(index{match})

	case api.View_Selector_Which_from:
		from, err := s.From()
		if err != nil {
			return query.Failure(err)
		}

		return query.From(index{from})
	}

	return query.Failuref("invalid selector: %s", s.Which())
}

// binds a record
type bindFunc func(routing.Record) error

func (s Server) bind(bind bindFunc, selector query.Selector) error {
	it, err := selector(s.Snapshot())
	if err != nil {
		return err
	}

	for r := it.Next(); r != nil; r = it.Next() {
		if err = bind(r); err != nil {
			break
		}
	}

	return err
}

func maybeRecord(call api.View_lookup) bindFunc {
	return func(r routing.Record) error {
		res, err := call.AllocResults()
		if err != nil {
			return err
		}

		return maybe(res, r)
	}
}

func iterator(ctx context.Context, call api.View_iter) bindFunc {
	stream := streamutil.New(ctx)
	handler := call.Args().Handler() // TODO(soon):  set up BBR here.

	return func(r routing.Record) error {
		call.Ack() // call lazily to provide backpressure
		return stream.Track(handler.Recv(ctx, record(r)))
	}
}

func record(r routing.Record) func(api.View_Handler_recv_Params) error {
	return func(ps api.View_Handler_recv_Params) error {
		rec, err := ps.NewRecord()
		if err != nil {
			return err
		}

		return copyRecord(rec, r)
	}
}

func maybe(res api.View_lookup_Results, r routing.Record) error {
	if r == nil {
		return nil
	}

	result, err := res.NewResult()
	if err != nil {
		return err
	}

	rec, err := result.NewJust()
	if err != nil {
		return err
	}

	return copyRecord(rec, r)
}

func copyRecord(rec api.View_Record, r routing.Record) error {
	if b, ok := r.(RecordBinder); ok {
		return b.BindRecord(rec)
	}

	if err := rec.SetPeer(string(r.Peer())); err != nil {
		return err
	}

	hb, err := rec.NewHeartbeat()
	if err != nil {
		return err
	}

	rec.SetSeq(r.Seq())
	pulse.Heartbeat{Heartbeat: hb}.SetTTL(r.TTL())
	hb.SetInstance(r.Instance())

	if err := copyHost(hb, r); err != nil {
		return err
	}

	return copyMeta(hb, r)
}

func copyHost(rec api.Heartbeat, r routing.Record) error {
	name, err := r.Host()
	if err == nil {
		err = rec.SetHost(name)
	}

	return err
}

func copyMeta(rec api.Heartbeat, r routing.Record) error {
	meta, err := r.Meta()
	if err == nil {
		err = rec.SetMeta(capnp.TextList(meta))
	}

	return err
}
