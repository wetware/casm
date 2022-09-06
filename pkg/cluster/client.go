package cluster

import (
	"context"
	"fmt"
	"sync"
	"time"

	"capnproto.org/go/capnp/v3"
	"github.com/libp2p/go-libp2p/core/peer"

	api "github.com/wetware/casm/internal/api/routing"
	casm "github.com/wetware/casm/pkg"
	"github.com/wetware/casm/pkg/cluster/pulse"
	"github.com/wetware/casm/pkg/cluster/routing"
)

type View api.View

func (v View) AddRef() View {
	return View(capnp.Client(v).AddRef())
}

func (v View) Release() {
	capnp.Client(v).Release()
}

// Lookup returns the first record to match the supplied query.
// Callers MUST call ReleaseFunc when finished. Note that this
// will invalidate any record returned by FutureRecord.
func (v View) Lookup(ctx context.Context, query Query) (FutureRecord, capnp.ReleaseFunc) {
	f, release := api.View(v).Lookup(ctx, func(ps api.View_lookup_Params) error {
		return query(ps)
	})

	return FutureRecord(f.Result()), release
}

// Iter returns an iterator that ranges over records matching
// the supplied query. Callers MUST call the ReleaseFunc when
// finished with the iterator.  Callers MUST NOT call methods
// on the iterator after calling the ReleaseFunc.
func (v View) Iter(ctx context.Context, query Query) (Iterator, capnp.ReleaseFunc) {
	var (
		h          = newHandler()
		f, release = api.View(v).Iter(ctx, h.Handler(query))
	)

	return Iterator{
		f: casm.Future(f),
		h: h,
	}, release
}

// Iterator is a stateful object that enumerates routing
// records.
//
// Callers SHOULD check the value of Err after a call to
// Next returns nil.  If Err() == nil and Next() == nil,
// the iterator is exhausted.
type Iterator struct {
	f casm.Future
	h *handler
}

// Err returns any error encountered by the iterator.
// If Err() != nil, future calls to Err() return the
// same error, and calls to Next() return nil.
//
// Callers SHOULD check Err() after a call to Next()
// returns nil.
func (it *Iterator) Err() error {
	select {
	case <-it.f.Done():
		return it.f.Err()
	default:
		return nil
	}
}

// Next upates the iterator's internal state and returns the
// next record in the stream.  If a call to Next returns nil,
// the iterator is exhausted.
//
// Records returned by Next are valid until the next call to
// Next, or until the iterator is released.  See View.Iter().
func (it *Iterator) Next() routing.Record {
	it.h.Sync()
	return it.h.Next()
}

type handler struct {
	send chan routing.Record
	sync chan struct{}
	once sync.Once
}

func newHandler() *handler {
	return &handler{
		send: make(chan routing.Record),
		sync: make(chan struct{}, 1),
	}
}

func (h *handler) Shutdown()            { close(h.send) }
func (h *handler) Sync()                { h.sync <- struct{}{} }
func (h *handler) Next() routing.Record { return <-h.send }

func (h *handler) Handler(query Query) func(api.View_iter_Params) error {
	return func(ps api.View_iter_Params) (err error) {
		if err = query(ps); err == nil {
			err = ps.SetHandler(api.View_Handler_ServerToClient(h))
		}

		return
	}
}

func (h *handler) Recv(ctx context.Context, call api.View_Handler_recv) error {
	rec, err := call.Args().Record()
	if err != nil {
		return err
	}

	r, err := newRecord(rec)
	if err != nil {
		return err
	}

	// inital call to it.Next()
	h.once.Do(func() {
		select {
		case <-h.sync:
		case <-ctx.Done():
		}
	})

	select {
	case h.send <- r:
	case <-ctx.Done():
		return ctx.Err()
	}

	select {
	case <-h.sync:
		return nil

	case <-ctx.Done():
		return ctx.Err()
	}
}

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

type clientRecord api.View_Record

func newRecord(rec api.View_Record) (clientRecord, error) {
	// validate record fields

	hb, err := rec.Heartbeat()
	if err != nil {
		return clientRecord{}, fmt.Errorf("heartbeat: %w", err)
	}

	if _, err := hb.Meta(); err != nil {
		return clientRecord{}, fmt.Errorf("meta: %w", err)
	}

	// use FooBytes to avoid allocating a string
	if _, err := hb.HostBytes(); err != nil {
		return clientRecord{}, fmt.Errorf("host: %w", err)
	}

	if _, err := rec.PeerBytes(); err != nil {
		return clientRecord{}, fmt.Errorf("peer:  %w", err)
	}

	return clientRecord(rec), nil
}

func (r clientRecord) Peer() peer.ID {
	id, _ := api.View_Record(r).Peer()
	return peer.ID(id)
}

func (r clientRecord) PeerBytes() ([]byte, error) {
	return api.View_Record(r).PeerBytes()
}

func (r clientRecord) Seq() uint64 {
	return api.View_Record(r).Seq()
}

func (r clientRecord) TTL() time.Duration {
	return r.heartbeat().TTL()
}

func (r clientRecord) Instance() uint32 {
	return r.heartbeat().Instance()
}

func (r clientRecord) Host() (string, error) {
	return r.heartbeat().Host()
}

func (r clientRecord) HostBytes() ([]byte, error) {
	return r.heartbeat().HostBytes()
}

func (r clientRecord) Meta() (routing.Meta, error) {
	return r.heartbeat().Meta()
}

func (r clientRecord) heartbeat() pulse.Heartbeat {
	hb, _ := api.View_Record(r).Heartbeat()
	return pulse.Heartbeat{Heartbeat: hb}
}

func (r clientRecord) BindRecord(rec api.View_Record) (err error) {
	if err = rec.SetPeer(string(r.Peer())); err == nil {
		rec.SetSeq(r.Seq())
		err = rec.SetHeartbeat(r.heartbeat().Heartbeat)
	}

	return
}
