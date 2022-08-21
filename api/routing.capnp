using Go = import "/go.capnp";

@0xc2974e3dc137fcee;

$Go.package("routing");
$Go.import("github.com/wetware/casm/internal/api/routing");

using Milliseconds = UInt32;


struct Heartbeat {
    # Heartbeat messages are used to implement an unstructured p2p
    # clustering service.  Hosts periodically emit heartbeats on a
    # pubsub topic (the "namespace") and construct a routing table
    # based on heartbeats received by other peers.
    #
    # Additional metadata can piggyback off of heartbeat messages,
    # allowing indexed operations on the routing table.

    ttl      @0 :Milliseconds;
    # Time-to-live, in milliseconds. The originator is considered
    # failed if a subsequent heartbeat is not received within ttl.
    
    instance @1 :UInt32;
    # An opaque identifier that uniquely distinguishes an instance
    # of a host. This identifier is randomly generated each time a
    # host boots.

    host     @2 :Text;
    # The hostname of the underlying host, as reported by the OS.
    # Users MUST NOT assume hostnames to be unique or non-empty.

    meta     @3 :List(Text);
    # A set of optional, arbitrary metadata fields.  Fields are
    # encoded as key-value pairs separated by the '=' rune.  Fields
    # are parsed into keys and values by splitting the string on the
    # first occurrenc of the '=' separator.  Subsequent occurrences
    # are treated as part of the value.
}
