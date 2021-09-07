using Go = import "/go.capnp";

@0xfa005a3c690f4a62;

$Go.package("boot");
$Go.import("github.com/wetware/casm/internal/api/boot");


struct MulticastPacket {
    union {
        query @0 :Text;  # namespace
        response @1 :Response;
    }

    struct Response {
        ns @0 :Text;
        signedEnvelope @1 :Data;  # peer.PeerRecord
    }
}
