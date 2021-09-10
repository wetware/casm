using Go = import "/go.capnp";

@0xbbd81c151780f030;

$Go.package("pex");
$Go.import("github.com/wetware/casm/internal/api/pex");


struct Gossip {
    hop @0 :UInt64;
    envelope @1 :Data;
}
