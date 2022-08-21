using Go = import "/go.capnp";

@0x86c7b3eb31eb86de;

$Go.package("testing");
$Go.import("github.com/wetware/casm/internal/api/testing");


interface Echoer {
    # Echoer is a simple interface that echoes a string back to
    # the caller.  It is intended for use in unit tests.

    echo @0 (payload :Text) -> (result :Text);
    # Echo returns the payload unchanged.
}


interface Streamer {
    # Streamer is a simple interface that facilitates tests involving
    # streaming capabilities.

    recv @0 () -> stream;
    # Recv is a trivial stream.
}
