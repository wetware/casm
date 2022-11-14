package debug

import (
	"context"
	"runtime/pprof"

	"capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/pogs"
	api "github.com/wetware/casm/internal/api/debug"
)

type Profile = api.Profile

const (
	ProfileCPU          = api.Profile_cpu
	ProfileAlloc        = api.Profile_allocs
	ProfileBlock        = api.Profile_block
	ProfileGoroutine    = api.Profile_goroutine
	ProfileHeap         = api.Profile_heap
	ProfileMutex        = api.Profile_mutex
	ProfileThreadCreate = api.Profile_threadcreate
	InvalidProfile      = ^api.Profile(0)
)

var DefaultProfiles = map[Profile]struct{}{
	ProfileCPU:          {},
	ProfileAlloc:        {},
	ProfileBlock:        {},
	ProfileGoroutine:    {},
	ProfileHeap:         {},
	ProfileMutex:        {},
	ProfileThreadCreate: {},
}

// ProfileFromString returns the named profile, defaulting to
// InvalidProfile if the named profile does not exist.
func ProfileFromString(name string) Profile {
	p := api.ProfileFromString(name)
	if p == 0 && name != Profile(0).String() {
		p = InvalidProfile
	}
	return p
}

// Debugger provides a set of tools for debugging live Wetware
// hosts.  The debugger can potentially reveal sensitive data,
// including cryptographic secrets and SHOULD NOT be provided
// to untrusted parties.
type Debugger api.Debugger

func (d Debugger) AddRef() Debugger {
	return Debugger(capnp.Client(d).AddRef())
}

func (d Debugger) Release() {
	capnp.Client(d).Release()
}

// SysInfo returns static data about the host.  This data is
// aggregated at host startup and does not change.
func (d Debugger) SysInfo(ctx context.Context, info *SysInfo) error {
	f, release := api.Debugger(d).SysInfo(ctx, nil)
	defer release()

	res, err := f.Struct()
	if err != nil {
		return err
	}

	si, err := res.SysInfo()
	if err == nil {
		err = bindSysInfo(info, si)
	}

	return err
}

// EnvVars returns the environment in the form "key=value".
func (d Debugger) EnvVars(ctx context.Context) ([]string, error) {
	f, release := api.Debugger(d).EnvVars(ctx, nil)
	defer release()

	res, err := f.Struct()
	if err != nil {
		return nil, err
	}

	env, err := res.EnvVars()
	if err != nil {
		return nil, err
	}

	return textListToSlice(env)
}

// Profiler returns an object capable of measuring the supplied profile.
// It is compatible with the Go pprof tool.  The returned capability is
// guaranteed to be a Sampler if p == CPUProfile, else it is guaranteed
// to be a Snapshotter.
func (d Debugger) Profiler(ctx context.Context, p Profile) (capnp.Client, capnp.ReleaseFunc) {
	bind := func(ps api.Debugger_profiler_Params) error {
		ps.SetProfile(p)
		return nil
	}

	f, release := api.Debugger(d).Profiler(ctx, bind)
	return f.Profiler(), release
}

// Tracer returns a tracer capability.  It is compatible with the Go pprof
// tool.
func (d Debugger) Tracer(ctx context.Context) (Sampler, capnp.ReleaseFunc) {
	f, release := api.Debugger(d).Tracer(ctx, nil)
	return Sampler(f.Tracer()), release
}

type Server struct {
	// Context contains static debugging information to provide via SysInfo.
	Context SystemContext

	// Environ returns a copy of strings representing the environment, in the
	// form "key=value".   This is usually an alias of os.Environ, but can be
	// overridden to filter out values, return a static array, etc.   Environ
	// is intended to avoid ambient authority.  If nil, Environ defaults to a
	// nil slice.
	Environ func() []string

	// Profiles is the set of profiles supported by the server instance.  It
	// usually contains the default profiles returned by pprof.Profiles, but
	// can be restricted or disabled entirely.
	Profiles map[Profile]struct{}
}

func (s Server) Debugger() Debugger {
	return Debugger(api.Debugger_ServerToClient(s))
}

func (s Server) Client() capnp.Client {
	return capnp.Client(s.Debugger())
}

func (s Server) SysInfo(_ context.Context, call api.Debugger_sysInfo) error {
	res, err := call.AllocResults()
	if err == nil {
		err = alloc(res.NewSysInfo, s.Context.Bind)
	}

	return err
}

func (s Server) EnvVars(_ context.Context, call api.Debugger_envVars) error {
	if s.Environ == nil {
		return nil
	}

	res, err := call.AllocResults()
	if err == nil {
		err = bindStrings(res.NewEnvVars, s.Environ())
	}

	return err
}

func (s Server) Profiler(_ context.Context, call api.Debugger_profiler) error {
	if _, ok := s.Profiles[call.Args().Profile()]; !ok {
		return nil
	}

	res, err := call.AllocResults()
	if err != nil {
		return err
	}

	var client capnp.Client
	switch p := call.Args().Profile(); p {
	case ProfileCPU:
		server := SamplingServer{Strategy: SampleCPU{}}
		client = server.Client()

	default:
		profile := pprof.Lookup(p.String())
		server := SnapshotServer{Profile: profile}
		client = server.Client()
	}

	return res.SetProfiler(client)

}

func (Server) Tracer(_ context.Context, call api.Debugger_tracer) error {
	res, err := call.AllocResults()
	if err == nil {
		server := SamplingServer{Strategy: Trace{}}
		err = res.SetTracer(api.Sampler(server.Sampler()))
	}

	return err
}

/*
	util
*/

func bindSysInfo(info *SysInfo, si api.SysInfo) error {
	return pogs.Extract(info, api.SysInfo_TypeID, si.ToPtr().Struct())
}

// bindStrings is a helper function that assigns a []string to a List(Text)
// field.  It returns any error resulting from the allocation of the capnp
// field, or assignment thereto.
func bindStrings(alloc func(int32) (capnp.TextList, error), ss []string) error {
	list, err := alloc(int32(len(ss)))
	if err == nil {
		err = copySliceToTextList(list, ss)
	}
	return err
}

func textListToSlice(list capnp.TextList) (ss []string, err error) {
	ss = make([]string, list.Len())
	for i := 0; i < list.Len(); i++ {
		if ss[i], err = list.At(i); err != nil {
			break
		}
	}
	return
}

func copySliceToTextList(list capnp.TextList, ss []string) (err error) {
	for i, s := range ss {
		if err = list.Set(i, s); err != nil {
			break
		}
	}
	return
}
