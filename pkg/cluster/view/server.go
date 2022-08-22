package view

import (
	"context"

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

	return s.bind(maybeRecord(call), selector(sel))
}

func (s Server) Iter(ctx context.Context, call api.View_iter) error {
	sel, err := call.Args().Selector()
	if err != nil {
		return err
	}

	return s.bind(iterator(ctx, call), selector(sel))
}

func selector(s api.View_Selector) query.Selector {
	switch s.Which() {
	case api.View_Selector_Which_match:
		match, err := s.Match()
		if err != nil {
			return query.Failure(err)
		}

		return query.Select(index{match}).Bind(query.First())

	case api.View_Selector_Which_range:
		min, err := s.Range().Min()
		if err != nil {
			return query.Failure(err)
		}

		max, err := s.Range().Max()
		if err != nil {
			return query.Failure(err)
		}

		return query.Range(index{min}, index{max})
	}

	return query.Failuref("invalid selector: %s", s.Which())
}

// binds a record
type binder func(routing.Record) error

func (s Server) bind(bind binder, selector query.Selector) error {
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

func maybeRecord(call api.View_lookup) binder {
	return func(r routing.Record) error {
		res, err := call.AllocResults()
		if err != nil {
			return err
		}

		return maybe(res, r)
	}
}

func iterator(ctx context.Context, call api.View_iter) binder {
	s := streamutil.New(ctx)
	// TODO(soon):  set up BBR here.
	return func(r routing.Record) error {
		return s.Track(call.Args().Handler().Recv(ctx, record(r)))
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
