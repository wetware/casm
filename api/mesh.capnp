using Go = import "/go.capnp";

@0xe71ccb83a9b1b95a;

$Go.package("mesh");
$Go.import("github.com/wetware/casm/internal/mesh");

interface Neighbor {
    interface Returner {
        return @0 (n :Neighbor) -> ();
    }

    walk @0 (r :Returner, depth :UInt8) -> ();
}
