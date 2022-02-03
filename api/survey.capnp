using Go = import "/go.capnp";
@0xef83879a531f9bf3;
$Go.package("survey");
$Go.import("github.com/wetware/casm/internal/api/survey");


struct SurveyRequest {
    src @0 :Data;
    distance @1 :UInt8;
    namespace @2 :Text;
}

struct SurveyResponse {
    namespace @0 :Text;
    envelope @1 :Data;
}

struct SurveyPacket {
    union {
        request @0 :SurveyRequest;
        response @1 :SurveyResponse;
    }  
}
