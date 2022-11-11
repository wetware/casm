package debug_test

import (
	"os/user"
	"runtime"
	"testing"

	"capnproto.org/go/capnp/v3"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	api "github.com/wetware/casm/internal/api/debug"
	casm "github.com/wetware/casm/pkg"
	"github.com/wetware/casm/pkg/debug"
)

func TestBindContext_zero(t *testing.T) {
	t.Parallel()

	var (
		cx   = debug.SystemContext{PID: -1}
		info = MustInfo(capnp.SingleSegment(nil))
	)

	err := cx.Bind(info)
	require.NoError(t, err,
		"should bind context to api.SysInfo")
	assert.False(t, info.HasAppVersion(),
		"AppVersion SHOULD NOT be set")
	assert.False(t, info.HasOs(),
		"os info SHOULD NOT be set")

	v, err := info.Version()
	require.NoError(t, err)
	assert.Equal(t, casm.Version, v,
		"CASM version SHOULD be set (got %s)", v)

	t.Run("Runtime", func(t *testing.T) {
		rt, err := info.Runtime()
		require.NoError(t, err)
		require.True(t, rt.IsValid(),
			"runtime info SHOULD be set")

		assert.Equal(t, runtime.NumCPU(), int(rt.NumCPU()),
			"unexpected number of CPUs")

		os, _ := rt.Os()
		assert.Equal(t, runtime.GOOS, os,
			"should be equal to runtime.GOOS")

		arch, _ := rt.Arch()
		assert.Equal(t, runtime.GOARCH, arch,
			"should be equal to runtime.GOARCH")

		comp, _ := rt.Compiler()
		assert.Equal(t, runtime.Compiler, comp,
			"should be equal to runtime.Compiler")
	})
}

func TestContext_OSinfo_Args(t *testing.T) {
	t.Parallel()

	var (
		info = MustInfo(capnp.SingleSegment(nil))
		cx   = debug.SystemContext{Argv: []string{"foo", "bar"}}
	)

	err := cx.Bind(info)
	require.NoError(t, err)

	osinfo, err := info.Os()
	require.NoError(t, err)

	as, err := osinfo.Args()
	require.NoError(t, err)

	args := make([]string, as.Len())
	for i := range args {
		args[i], err = as.At(i)
		require.NoError(t, err)
	}

	assert.Equal(t, cx.Argv, args)
}

func TestContext_UserInfo_UID(t *testing.T) {
	t.Parallel()
	t.Helper()

	var (
		info = MustInfo(capnp.SingleSegment(nil))
		cx   debug.SystemContext
	)

	for _, tt := range []struct {
		name, uid string
		want      api.OSInfo_user_uid_Which
	}{
		{name: "Withheld", uid: "", want: api.OSInfo_user_uid_Which_none},
		{name: "Unknown", uid: "-1", want: api.OSInfo_user_uid_Which_none},
		{name: "Numeric/Kernel", uid: "0", want: api.OSInfo_user_uid_Which_numeric},
		{name: "Numeric/User", uid: "1", want: api.OSInfo_user_uid_Which_numeric},
		{name: "Token/Text", uid: "foo", want: api.OSInfo_user_uid_Which_token},
		{name: "Token/Symbol", uid: "-f0∂", want: api.OSInfo_user_uid_Which_token},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cx.User = &user.User{Uid: tt.uid}

			err := cx.Bind(info)
			require.NoError(t, err)

			osinfo, err := info.Os()
			require.NoError(t, err)

			got := osinfo.User().Uid().Which()
			require.Equal(t, tt.want, got,
				"expected %s, got %s", tt.want, got)
		})
	}
}

func TestContext_UserInfo_GID(t *testing.T) {
	t.Parallel()
	t.Helper()

	var (
		info = MustInfo(capnp.SingleSegment(nil))
		cx   debug.SystemContext
	)

	for _, tt := range []struct {
		name, gid string
		want      api.OSInfo_user_gid_Which
	}{
		{name: "Withheld", gid: "", want: api.OSInfo_user_gid_Which_none},
		{name: "Unknown", gid: "-1", want: api.OSInfo_user_gid_Which_none},
		{name: "Numeric/Kernel", gid: "0", want: api.OSInfo_user_gid_Which_numeric},
		{name: "Numeric/User", gid: "1", want: api.OSInfo_user_gid_Which_numeric},
		{name: "Token/Text", gid: "foo", want: api.OSInfo_user_gid_Which_token},
		{name: "Token/Symbol", gid: "-f0∂", want: api.OSInfo_user_gid_Which_token},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cx.User = &user.User{Gid: tt.gid}

			err := cx.Bind(info)
			require.NoError(t, err)

			osinfo, err := info.Os()
			require.NoError(t, err)

			got := osinfo.User().Gid().Which()
			require.Equal(t, tt.want, got,
				"expected %s, got %s", tt.want, got)
		})
	}
}

func MustInfo(a capnp.Arena) api.SysInfo {
	info, err := NewInfo(a)
	if err != nil {
		panic(err)
	}
	return info
}

func NewInfo(a capnp.Arena) (api.SysInfo, error) {
	_, seg, err := capnp.NewMessage(a)
	if err != nil {
		return api.SysInfo{}, err
	}

	return api.NewSysInfo(seg)
}
