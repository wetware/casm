using Go = import "/go.capnp";

@0xbbd81c151780f030;

$Go.package("pex");
$Go.import("github.com/wetware/casm/internal/api/pex");


struct Gossip {
    # Gossip is a PeX cache entry, containing addressing
    # information for a cluster peer.

    hop @0 :UInt64;
    # hop is a counter that increases monotonically each
    # time the Gossip record is received.  It is used to
    # determine the "network age" of the record.
    #
    # Hop MUST be set to zero for the last Gossip record
    # in a stream.  All other Gossip records in a stream
    # must have hop > 1.

    envelope @1 :Data;
    # signed envelope containing a lip2p PeerRecord.  The
    # envelope of the last Gossip record in a stream MUST
    # contain the sender's peer record. Envelopes MUST be
    # signed by the key matching the PeerRecord's peer.ID.
}
