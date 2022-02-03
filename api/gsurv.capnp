using Go = import "/go.capnp";
@0xef83879a531f9bf3;
$Go.package("gsurv");
$Go.import("github.com/wetware/casm/internal/api/gsurv");


struct GSurvRequest {
    src @0 :Data;
    distance @1 :UInt8;
    namespace @2 :Text;
}

struct GSurvResponse {
    namespace @0 :Text;
    envelope @1 :Data;
}

struct GSurvPacket {
    union {
        request @0 :GSurvRequest;
        response @1 :GSurvResponse;
    }  
}
