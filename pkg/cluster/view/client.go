package view

import (
	"context"
	"fmt"

	"capnproto.org/go/capnp/v3"

	"github.com/libp2p/go-libp2p-core/peer"
	api "github.com/wetware/casm/internal/api/routing"
	"github.com/wetware/casm/pkg/cluster/pulse"
	"github.com/wetware/casm/pkg/cluster/routing"
)

type View api.View

func (v View) Client() capnp.Client {
	return capnp.Client(v)
}

func (v View) AddRef() View {
	return View(v.Client().AddRef())
}

func (v View) Release() {
	v.Client().Release()
}

func (v View) Lookup(ctx context.Context, query Query) (FutureRecord, capnp.ReleaseFunc) {
	f, release := api.View(v).Lookup(ctx, func(ps api.View_lookup_Params) error {
		return query(ps)
	})

	return FutureRecord(f.Result()), release
}

// func (v View) Iter(ctx context.Context, query Query) (Iterator, capnp.ReleaseFunc) {
// 	f, release := api.View(v).Iter(ctx, func(ps api.View_iter_Params) error {
// 		// XXX:  SET UP HANDLER HERE

// 		return query(ps)
// 	})

// 	return Iterator{
// 		// ...
// 	}, release
// }

// type Iterator struct {
// 	// ...
// }

// func (it Iterator) Err() error {

// }

// func (it Iterator) Next() routing.Record {

// }

type FutureRecord api.View_MaybeRecord_Future

func (f FutureRecord) Await(ctx context.Context) (routing.Record, error) {
	select {
	case <-f.Done():
		return f.Record()

	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (f FutureRecord) Record() (routing.Record, error) {
	res, err := api.View_MaybeRecord_Future(f).Struct()
	if err != nil {
		return nil, err
	}

	if !res.HasJust() {
		return nil, nil // no record
	}

	rec, err := res.Just()
	if err != nil {
		return nil, err
	}

	return newRecord(rec)
}

type clientRecord struct {
	id  peer.ID
	seq uint64
	pulse.Heartbeat
}

func newRecord(rec api.View_Record) (routing.Record, error) {
	id, err := rec.Peer()
	if err != nil {
		return nil, fmt.Errorf("peer:  %w", err)
	}

	hb, err := rec.Heartbeat()
	if err != nil {
		return nil, fmt.Errorf("heartbeat: %w", err)
	}

	return &clientRecord{
		id:        peer.ID(id),
		seq:       rec.Seq(),
		Heartbeat: pulse.Heartbeat{Heartbeat: hb},
	}, nil
}

func (r clientRecord) Peer() peer.ID { return r.id }
func (r clientRecord) Seq() uint64   { return r.seq }

func (r clientRecord) BindRecord(rec api.View_Record) (err error) {
	if err = rec.SetPeer(string(r.Peer())); err == nil {
		rec.SetSeq(r.Seq())
		err = rec.SetHeartbeat(r.Heartbeat.Heartbeat)
	}

	return
}
