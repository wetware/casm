using Go = import "/go.capnp";

@0xc2974e3dc137fcee;

$Go.package("cluster");
$Go.import("github.com/wetware/casm/internal/api/cluster");


# Announcements are broadcast over the cluster's pubsub topic.
struct Announcement {
    # Heartbeat messages are periodically broadcast in a pubsub
    # topic whose name is the cluster's namespace string. This
    # is used to track the liveness of peers in a cluster, as
    # well as build a routing table so that peers can connec to
    # each other.  User-defined metadata can piggyback off of
    # these messages.  A common case is to include the node's
    # hostname.
    struct Heartbeat {
        # Time-to-live:  the duration of time during which the
        # heartbeat is to be considered valid.
        ttl @0 :Int64;

        # Heartbeat messages may contain arbitrary metadata.
        #
        # - Use 'metaString' when sending human-readable values,
        #   or text-encoded data like JSON and XML.
        # - Use 'metaBytes' when sending non-capnp binary data,
        #   for example using CBOR, MSGPACK or Protocol Buffers.
        #
        # This data will be broadcast to all pears at each heartbeat,
        # so users are encoraged to be very terse.
        record :union {
            none @1 :Void;
            text @2 :Text;
            binary @3 :Data;
            pointer @4 :AnyPointer;
        }
    }

    # JoinLeave announcements indicates that a peer's neighbor
    # believes the peer to have left the cluster.  This should
    # be treated with caution because it might instead reflect
    # reachability issues between those two peers.
    using JoinLeave = Text;

    union {
        heartbeat @0 :Heartbeat;
        join @1 :JoinLeave;
        leave @2 :JoinLeave;
    }
}
