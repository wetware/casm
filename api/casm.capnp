using Go = import "/go.capnp";

@0x8649a65772a7be4a;

$Go.package("casm");
$Go.import("github.com/wetware/casm/internal/api/casm");


struct Envelope {
    publicKey   @0 :Data;
    payloadType @1 :Data;
    rawPayload  @2 :Data;
}


interface Scanner {
    scan @0 (envelope :Envelope) -> ();
}

interface Loader {
    load @0 () -> (envelope :Envelope);
}
