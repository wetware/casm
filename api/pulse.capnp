using Go = import "/go.capnp";

@0xc2974e3dc137fcee;

$Go.package("pulse");
$Go.import("github.com/wetware/casm/internal/api/pulse");


struct Heartbeat {
    # Heartbeat messages are used to implement an unstructured p2p
    # clustering service.  Hosts periodically emit heartbeats on a
    # pubsub topic (the "namespace") and construct a routing table
    # based on heartbeats received by other peers.
    #
    # Additional metadata can piggyback off of heartbeat messages,
    # allowing indexed operations on the routing table.

    ttl      @0 :UInt32;
    # Time-to-live, in milliseconds. The originator is considered
    # failed if a subsequent heartbeat is not received within ttl.
    
    id       @1 :UInt32;
    # An opaque identifier that uniquely distinguishes an instance
    # of a host. This identifier is randomly generated each time a
    # host boots.

    hostname @2 :Text;
    # The hostname of the underlying host, as reported by the OS.
    # Users MUST NOT assume hostnames to be unique or non-empty.

    meta     @3 :List(Field);
    # A set of optional, arbitrary metadata fields.  The total size
    # of meta SHOULD be kept as small as possible.

    struct Field {
        key   @0 :Text;
        value @1 :Text;
    }
}
