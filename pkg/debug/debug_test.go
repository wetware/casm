package debug_test

import (
	"context"
	"testing"

	"capnproto.org/go/capnp/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	casm "github.com/wetware/casm/pkg"
	"github.com/wetware/casm/pkg/debug"
)

func TestSysInfo(t *testing.T) {
	t.Parallel()

	var info debug.SysInfo
	server := debug.Server{
		Context: debug.SystemContext{
			Version: "1.2.3",
		},
	}

	err := server.Debugger().SysInfo(context.Background(), &info)
	require.NoError(t, err)

	assert.Equal(t, casm.Version, info.CASMVersion,
		"unexpected value in field CASMVersion")
	assert.Equal(t, server.Context.Version, info.Version,
		"unexpected value in field CASMVersion")
}

func TestDebugger_EnvVars(t *testing.T) {
	t.Parallel()
	t.Helper()

	t.Run("NotProvided", func(t *testing.T) {
		t.Parallel()

		args, err := debug.Server{}.Debugger().EnvVars(context.Background())
		assert.NoError(t, err, "rpc should not fail")
		assert.Empty(t, args, "should not return environment")
	})

	t.Run("Provided", func(t *testing.T) {
		t.Parallel()

		want := []string{"foo", "bar", "baz"}
		s := debug.Server{Environ: func() []string { return want }}

		args, err := s.Debugger().EnvVars(context.Background())
		assert.NoError(t, err, "rpc should not fail")
		assert.Equal(t, want, args, "should match environment from server")
	})
}

func TestDebugger_Profiler(t *testing.T) {
	t.Parallel()
	t.Helper()

	t.Run("NotProvided", func(t *testing.T) {
		t.Parallel()

		d := debug.Server{}.Debugger()
		p, release := d.Profiler(context.Background(), debug.ProfileCPU)
		assert.NotNil(t, release, "should return capnp.ReleaseFunc")
		assert.True(t, nullcap(p), "should return null capability")
	})

	t.Run("ProvideDefault", func(t *testing.T) {
		t.Parallel()

		d := debug.Server{Profiles: debug.DefaultProfiles}.Debugger()
		for profile := range debug.DefaultProfiles {
			p, release := d.Profiler(context.Background(), profile)
			assert.NotNil(t, release, "should return capnp.ReleaseFunc")
			assert.False(t, nullcap(p), "should provide %s", profile)
		}
	})
}

func TestDebugger_Tracer(t *testing.T) {
	t.Parallel()
	t.Helper()

	d := debug.Server{Profiles: debug.DefaultProfiles}.Debugger()
	p, release := d.Tracer(context.Background())
	assert.NotNil(t, release, "should return capnp.ReleaseFunc")
	assert.False(t, nullcap(p), "should provide tracer")
}

func nullcap[T ~capnp.ClientKind](t T) bool {
	client := capnp.Client(t)
	_ = client.Resolve(context.Background())
	return client.IsSame(capnp.Client{})
}
