package debug

import (
	"os/user"
	"runtime"
	"strconv"

	api "github.com/wetware/casm/internal/api/debug"
	casm "github.com/wetware/casm/pkg"
)

type SysInfo struct {
	// Version contains an application-defined version string.
	// Semantic versioning is RECOMMENDED: https://semver.org.
	Version string `json:"app_version" capnp:"appVersion"`

	// CASM contains a version string for the CASM runtime. It
	// adheres to the Semantic Versioning v2.0.0 standard.
	CASMVersion string `json:"casm_version" capnp:"version"`

	Runtime *RuntimeInfo `json:"runtime,omitempty"`
	OS      *OSInfo      `json:"os,omitempty" capnp:"os"`
}

type RuntimeInfo struct {
	GOOS     string `capnp:"os"`
	GOARCH   string `capnp:"arch"`
	Compiler string `json:"compiler,omitempty"`
	NumCPU   uint32 `json:"num_cpu,omitempty"`
}

type OSInfo struct {
	PID      int64     `json:"pid" capnp:"pid"`
	Hostname string    `json:"hostname,omitempty"`
	Args     []string  `json:"args,omitempty"`
	User     *UserInfo `json:"user,omitempty"`
}

type UserInfo struct {
	Username    string `json:"username,omitempty"`
	DisplayName string `json:"display_name,omitempty" capnp:"displayName"`
	HomeDir     string `json:"home_dir,omitempty" capnp:"homeDir"`
}

// SystemContext contains information about the host used to
// populate SysInfo. For security reasons, the debug package
// prohibits system calls and references to global variables,
// instead requiring applications to provide data explicitly
// by means of the SystemContext. This helps prevent ambient
// authority attacks.
type SystemContext struct {
	// Version contains an application-defined version string.
	// Semantic versioning is RECOMMENDED: https://semver.org.
	Version string

	// Argv contains the command-line arguments used to start the
	// CASM application.  In most applications, Argv is populated
	// by the os.Args global, but developers MAY populate Argv by
	// other means, or omit them entirely.
	Argv []string

	// PID contains the application's process ID.  If the PID is
	// unknown or withheld, applications SHOULD set PID to -1.
	PID int

	// Hostname of the vat.  In most applications, this contains
	// the output of os.Hostname, but developers MAY populate it
	// through other means.
	Hostname string

	// User account under which the process is running.  In most
	// applications, this is obtained via os/user.Current(), but
	// developers MAY populate this argument by other means. Nil
	// values are ignored.
	User *user.User
}

func (cx *SystemContext) Bind(info api.SysInfo) error {
	return bind(info,
		version[api.SysInfo](casm.Version),
		cx.appVersion,
		runtimeInfo,
		cx.osInfo)
}

func (cx *SystemContext) appVersion(info api.SysInfo) error {
	return info.SetAppVersion(cx.Version)
}

func bindRuntimeInfo(info api.RuntimeInfo) error {
	// NumCPU effectively behaves as a constant, so it's safe to
	// call directly.  See docstring for more details.
	info.SetNumCPU(uint32(runtime.NumCPU()))

	return bind(info,
		compiler,
		architecture,
		operatingSystem)
}

func architecture(info api.RuntimeInfo) error {
	// runtime.GOARCH is a constant, so it's safe to reference.
	return info.SetArch(runtime.GOARCH)
}

func compiler(info api.RuntimeInfo) error {
	// runtime.Compiler is a constant, so it's safe to reference.
	return info.SetCompiler(runtime.Compiler)
}

func operatingSystem(info api.RuntimeInfo) error {
	return info.SetOs(runtime.GOOS)
}

func runtimeInfo(info api.SysInfo) error {
	return alloc(info.NewRuntime, bindRuntimeInfo)
}

func (cx *SystemContext) bindOSInfo(info api.OSInfo) error {
	return bind(info,
		cx.pid,
		cx.hostname,
		cx.args,
		cx.userInfo)
}

func (cx *SystemContext) osInfo(info api.SysInfo) (err error) {
	hasHost := cx.Hostname != ""
	hasProc := cx.PID >= 0 || len(cx.Argv) > 0 || cx.User != nil

	if hasHost || hasProc {
		err = alloc(info.NewOs, cx.bindOSInfo)
	}

	return
}

func (cx *SystemContext) hostname(info api.OSInfo) error {
	return info.SetHostname(cx.Hostname)
}

func (cx *SystemContext) args(info api.OSInfo) error {
	args, err := info.NewArgs(int32(len(cx.Argv)))
	if err == nil {
		err = copySliceToTextList(args, cx.Argv)
	}

	return err
}

func (cx *SystemContext) pid(info api.OSInfo) error {
	info.SetPid(int64(cx.PID))
	return nil
}

func (cx *SystemContext) userInfo(info api.OSInfo) (err error) {
	if cx.User != nil {
		err = bind(info.User(),
			cx.userLogin,
			cx.displayName,
			cx.homeDir,
			cx.userID,
			cx.groupID)
	}

	return
}

func (cx *SystemContext) userLogin(info api.OSInfo_user) error {
	return info.SetUsername(cx.User.Username)
}

func (cx *SystemContext) displayName(info api.OSInfo_user) error {
	return info.SetDisplayName(cx.User.Name)
}

func (cx *SystemContext) homeDir(info api.OSInfo_user) error {
	return info.SetHomeDir(cx.User.HomeDir)
}

func (cx *SystemContext) userID(info api.OSInfo_user) error {
	if cx.User.Uid == "" {
		return nil
	}

	uid, err := strconv.Atoi(cx.User.Uid)
	if uid >= 0 && err == nil {
		info.Uid().SetNumeric(uint64(uid))
	} else if err != nil {
		err = info.Uid().SetToken(cx.User.Uid)
	}

	return err
}

func (cx *SystemContext) groupID(info api.OSInfo_user) error {
	if cx.User.Gid == "" {
		return nil
	}

	uid, err := strconv.Atoi(cx.User.Gid)
	if uid >= 0 && err == nil {
		info.Gid().SetNumeric(uint64(uid))
	} else if err != nil {
		err = info.Gid().SetToken(cx.User.Gid)
	}

	return err
}

func version[T interface{ SetVersion(string) error }](v string) func(T) error {
	return func(info T) error {
		return info.SetVersion(v)
	}
}

func alloc[T any](f func() (T, error), bind func(T) error) error {
	t, err := f()
	if err == nil {
		err = bind(t)
	}
	return err
}

func bind[T any](t T, fs ...func(T) error) (err error) {
	for _, bind := range fs {
		if err = bind(t); err != nil {
			break
		}
	}

	return
}
