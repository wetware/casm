using Go = import "/go.capnp";
@0xef83879a531f9bf3;
$Go.package("survey");
$Go.import("github.com/wetware/casm/internal/api/survey");


struct Packet {
    namespace @0 :Text;

    union {
        request @1 :Request;
        response @2 :Data;  # marshaled envelope
    }  

    struct Request {
        src @0 :Data;
        distance @1 :UInt8;
    }
}
