using Go = import "/go.capnp";

@0xe71ccb83a9b1b95a;

$Go.package("mesh");
$Go.import("github.com/wetware/casm/internal/mesh");

interface Edge {
    interface Returner {
        return @0 (n :Edge) -> ();
    }

    walk @0 (r :Returner, depth :UInt8) -> ();
}
