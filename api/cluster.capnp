using Go = import "/go.capnp";

@0xc2974e3dc137fcee;

$Go.package("cluster");
$Go.import("github.com/wetware/casm/internal/api/cluster");


struct Announcement {
    instance @0 :UInt64;
    union {
        peerLeaving @1 :Void;
        ttl @2 :Int64;
    }
}
