using Go = import "/go.capnp";
@0xef83879a531f9bf3;
$Go.package("mudp");
$Go.import("github.com/wetware/casm/internal/api/mudp");


struct MudpRequest {
    src @0 :Data;
    distance @1 :UInt8;
    namespace @2 :Text;
}

struct MudpResponse {
    namespace @0 :Text;
    envelope @1 :Data;
}

struct MudpPacket {
    union {
        request @0 :MudpRequest;
        response @1 :MudpResponse;
    }  
}