using Go = import "/go.capnp";

@0xc2974e3dc137fcee;

$Go.package("pulse");
$Go.import("github.com/wetware/casm/internal/api/pulse");


# Cluster events are are broadcast over the cluster's pubsub topic.
# There are t
struct Event {
    using PeerID = Text;

    # Heartbeat messages are periodically broadcast in a pubsub
    # topic whose name is the cluster's namespace string. This
    # is used to track the liveness of peers in a cluster, as
    # well as to build a routing table so that peers can connect
    # to each other.  User-defined metadata can piggyback off of
    # these messages.  A common case is to include the node's
    # hostname.
    struct Heartbeat {
        # Time-to-live:  the duration of time during which the
        # heartbeat is to be considered valid.
        ttl @0 :Int64;

        # Heartbeat messages may contain arbitrary metadata.
        #
        # - Use 'text' when sending human-readable values, or
        #   text-encoded data like JSON and XML.
        # - Use 'binary' when sending non-capnp binary data,
        #   for example using CBOR, MSGPACK or Protocol Buffers.
        #
        # - Use 'pointer' to transmit an arbitrary Cap'n Proto
        #   pointer type.
        #
        # This data will be broadcast to all pears at each heartbeat,
        # so users are encoraged to be very terse.
        meta :union {
            none @1 :Void;
            text @2 :Text;
            binary @3 :Data;
            pointer @4 :AnyPointer;
        }
    }

    # Join and Leave are emitted by peers when they believe one of their
    # neighbors to have joined or left the cluster.  Such events should
    # be treated with caution because both are prone to false positives.
    # A peer p considers a neighbor n to have joined if (1) n has just
    # connected to p and (2) n is not currently in p's routing table.
    # Conversely, n is considered to have left only if the Leave event
    # was emitted by n itself.  This is because peers routinely disconnect
    # for legitimate reasons, such as topology management.
    #
    # TODO(enhancement):  p should respond to disconnections by sending a
    #                     ping message to n, and should emit a Leave event
    #                     if n cannot be reached.
    union {
        heartbeat @0 :Heartbeat;
        join @1 :PeerID;
        leave @2 :PeerID;
    }
}
