using Go = import "/go.capnp";

@0xa90d3b8939007f97;

$Go.package("debug");
$Go.import("github.com/wetware/casm/internal/api/debug");

enum Profile {
    # Profile is an enum that maps onto the default profiles
    # in runtime/pprof.

    cpu          @0;  # sample-based profile
    allocs       @1;
    block        @2;
    goroutine    @3;
    heap         @4;
    mutex        @5;
    threadcreate @6;
}


interface Debugger {
    # Debugger provides a set of tools for debugging live Wetware
    # hosts.  The debugger can potentially reveal sensitive data,
    # including cryptographic secrets and SHOULD NOT be provided
    # to untrusted parties.

    sysInfo  @0 () -> (sysInfo :SysInfo);
    # SysInfo returns static information about the Wetware host.
    # Data returned by SysInfo is collected statically when the
    # host starts, so it need not be called more than once.

    envVars  @1 () -> (envVars :List(Text));
    # EnvVars returns the host's current environment in the form
    # "key=value".   Contrary to the SysInfo data, data returned
    # from EnvVars will reflect changes to the environment.

    profiler @2 (profile :Profile) -> (profiler :Capability);
    # Profiler returns an object capable of measuring the supplied 
    # profile. The snapshot data is formatted in Go's pprof format.
    # If profile == cpu, the resulting capability is guaranteed to
    # be a Sampler.  Otherwise, it is a Shapshotter.  The duration
    # parameter is assigned to the CPU profiler, or ignored.

    tracer   @3 () -> (tracer :Sampler);
    # Tracer returns a sampler that performs a live trace on the
    # host vat. Trace data is streamed to a Writer in Go's pprof
    # format.
}


struct SysInfo {
    # SysInfo contains static information about the CASM app.

    version    @0 :Text;        # CASM version
    appVersion @1 :Text;        # application version
    runtime    @2 :RuntimeInfo;
    os         @3 :OSInfo;
}


struct RuntimeInfo {
    # RuntimeInfo contains information about the Go runtime.

    numCPU   @0 :UInt32;
    # NumCPU returns the number of logical CPUs usable by the
    # vat process.    The set of available CPUs is checked by
    # querying the OS at process startup.   Changes to OS CPU
    # allocation after process startup are not reflected.

    os       @1 :Text;
    arch     @2 :Text;
    compiler @3 :Text;
}

struct OSInfo {
    # OSInfo contains information about the host operating system.
    
    pid             @0  :Int64;
    # PID contains the vat process's numeric ID.

    hostname        @1  :Text;
    # Hostname contains the host name reported by the kernel.

    args            @2  :List(Text);
    # Args hold the command-line arguments, starting with
    # the program name.

    user                :group {
    # User account under which the vat process is executing.
        
        username    @3  :Text;
        # Username is the login name.

        displayName @4  :Text;
        # DisplayName is the user's real, or "display" name.  It MAY
        # be blank. On POSIX systems, this is the first entry in the
        # GECOS field list. On Windows, it is the user's DisplayName.
        # On Plan 9, this is the contents of /dev/user.

        homeDir     @5  :Text;
        # HomeDir is the path to the user's home directory.  If there
        # is no home directory for the user, it is left blank.

        uid             :union {
        # ID of the user.  On POSIX systems, this is a numeric value
        # representing the UID.   On Windows, this is a string token
        # containig a security identifier (SID).  On Plan 9, this is
        # a string token containing the contents of /dev/user.

            none    @6  :Void;
            numeric @7  :UInt64;
            token   @8  :Text;
        }

        gid             :union {
        # GID for the user's primary group. On POSIX systems, this is
        # a decimal number.
            none    @9  :Void;
            numeric @10 :UInt64;
            token   @11 :Text;
        }
    }
}


interface Snapshotter {
    snapshot @0 (debug :UInt8) -> (snapshot :Data);
}


interface Sampler {
    sample @0 (writer :Writer, duration :Int64) -> ();

    interface Writer {
        write @0 (sample :Data) -> stream;
    }
}
