package debug

import (
	"bytes"
	"context"
	"errors"
	"runtime/pprof"

	"capnproto.org/go/capnp/v3"
	api "github.com/wetware/casm/internal/api/debug"
)

var ErrProfileNotFound = errors.New("profile not found")

type Snapshotter api.Snapshotter

func (s Snapshotter) AddRef() Snapshotter {
	return Snapshotter(capnp.Client(s).AddRef())
}

func (s Snapshotter) Release() {
	capnp.Client(s).Release()
}

func (s Snapshotter) Snapshot(ctx context.Context, debug uint8) ([]byte, error) {
	bind := func(ps api.Snapshotter_snapshot_Params) error {
		ps.SetDebug(debug)
		return nil
	}

	f, release := api.Snapshotter(s).Snapshot(ctx, bind)
	defer release()

	res, err := f.Struct()
	if err != nil {
		return nil, err
	}

	return res.Snapshot()
}

// SnapshotServer is a profiler that takes a non-blocking snapshot of
// process state using pprof.Profile.
type SnapshotServer struct {
	Profile *pprof.Profile
}

func (s SnapshotServer) Snapshotter() Snapshotter {
	return Snapshotter(api.Snapshotter_ServerToClient(s))
}

func (s SnapshotServer) Client() capnp.Client {
	return capnp.Client(s.Snapshotter())
}

func (s SnapshotServer) Snapshot(_ context.Context, call api.Snapshotter_snapshot) error {
	if s.Profile == nil {
		return ErrProfileNotFound
	}

	var buf bytes.Buffer
	if err := s.Profile.WriteTo(&buf, 0); err != nil {
		return err
	}

	res, err := call.AllocResults()
	if err == nil {
		err = res.SetSnapshot(buf.Bytes())
	}

	return err
}
