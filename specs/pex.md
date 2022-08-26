# Peer Exchange (PeX)

Gossip-based sampling service for robust connectivity.


## Table of Contents

- [Peer Exchange (PeX)](#peer-exchange-pex)
  - [Table of Contents](#table-of-contents)
  - [Motivation](#motivation)
- [Protocol Specification](#protocol-specification)
  - [Conventions](#conventions)
  - [Definitions](#definitions)
  - [Description](#description)
  - [Record Age](#record-age)
  - [Peer Selection](#peer-selection)
  - [Push-Pull](#push-pull)
  - [View Merging](#view-merging)
    - [Record Deduplication](#record-deduplication)
    - [Parameters](#parameters)
- [API](#api)
  - [Stream Identifier](#stream-identifier)
  - [Gossip Record](#gossip-record)
- [Known Issues](#known-issues)
  - [Authors](#authors)
  - [References](#references)

## Motivation

When a CASM process is started, its first order of business is to join a cluster.  This is done by establishing a connection with at least one peer already in the cluster, which presents an obvious bootstrapping problem:  how does one discover the initial peer?

There exist many solutions — CASM for example supports the following modes of bootstrap discovery: IP crawling, UDP multicast, static bootstrap nodes, and federated services — but each is limited in its ability to scale.  Indeed, peer-to-peer systems like CASM were invented precisely to circumvent the scalability limits of such systems.  Thus, a key idea in CASM's design is that bootstrap services should be used as infrequently as possible.  Ideally, each node would only query its bootstrap service at first startup, and then rely on the cluster itself to dicover additional peers.

In reality, this proves easier said than done.  CASM is designed with datacenter, peer-to-peer, edge, mobile and IoT applications in mind.  Hosts are expected to perform reliably in the presence of network outages, roaming and network partitions, all of which can cause a host to become isolated from the rest of the cluster, at which point it must bootstrap again.

PeX solves this problem by providing each host with a “recovery cache”: a small, continuously-updated list of peers that can be used for partition repair and subsequent bootstrapping.  It is provided as an unstructed peer-sampling service<sup>2</sup> that updates this cache in the background with little overhead.

Key properties of the PeX protocol are:

1. **Partition-Resistance.**  The cache remains valid in the presence of network partitions.  The speed at which peers in one partiton "forget" about peers in another is controllable.

2. **Persistence.**  Items remain in the cache until they have been explicitly evicted or replaced with suitable alternatives.  Explicit TTLs are avoided, since these can ossify partitions.

3. **Liveness.**  Changes to host addresses are propagated through the network quickly, and dead peers are eventually evicted from cache.

4. **Unbiased Sampling.**  Peers are be equally likely to appear in all cache instances.  This is essential to prevent the formation of partitions and to avoid overloading peers when partitions merge.

5. **Scalability.**  The global system should stabilize in better-than-linear time.  Resource usage should increase sub-linearly with respect to cluster size.

6. **Churn Tolerance.**  The cache is unaffected by repeated cluster merges and the protocol cannot enter an invalid state under such conditions.

# Protocol Specification

## Conventions

>The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT", "SHOULD", "SHOULD NOT", "RECOMMENDED",  "MAY", and "OPTIONAL" in this document are to be interpreted as described in [RFC 2119](https://datatracker.ietf.org/doc/html/rfc2119) and [RFC 2119](https://datatracker.ietf.org/doc/html/rfc8174).

## Definitions
- **Overlay**: a network formed by references to peers in the ensemble of caches across all peers.  The overlay is a graph structure in which each vertex is a host, and a link between two hosts exists iff host A contains host B in its cache.
- **Peer**: a physical host in the overlay.  Synonymous with node.
- **Neighbor/Neighborhod**: adjacent nodes in the overlay.  A node's neighborhood is the set of its neighbors.
- **View**:  The set of set of cache entries for a particular peer.
- **Gossip**: a broadcast mechanism in which peers relay messages to their neighbors.
- **Gossip Record**:  synonym of "cache entry".  Contains addressing information for a particular host, along with an age field and a sequence number.
- **Gossip Round**:  a single exchange of gossip with a neighbor.
- **Merge Policy**: in the context of a gossip round, this refers to strategy for selecting a new view from the union of one's local view and a view received from a peer.
- **Eviction**:  the process of removing an entry from the local view of a peer.  The term "eviction" may also be applied in the context of an entire cluster to refer to the process by which an accumulation of evictions by individual nodes results in the record being purged from a given partition.

## Description

At its core, PeX is a peer-sampling service that provides clients with a uniformly-random subset of peer addresses.  It adheres to libp2p's [Discovery](https://github.com/libp2p/go-libp2p/core/blob/master/discovery/discovery.go) API, allowing it to integrate seemlessly with existing applications.  In particular, it is suitable for use with PubSub, which powers CASM's core clustering service.

PeX maintains a bounded cache of peer addresses.  Updates to the cache are performed by gossipping with peers currently in the cache.  The result is an unstructured overlay network, similar to the one described in *[Gossip-Based Peer Sampling](https://dl.acm.org/doi/abs/10.1145/1275517.1275520)*.

Gossiping can be well understood by analogy.  Imagine a high-school where students talk to each other in the hall whenever they have a break. Each student encounters other randomly, and shares the rumour.  If a new rumour is first told in the morning, by the end of the day, almost every student will have heard about it.

PeX operates on a similar principle to ensure that information about peers is evenly distributed across hosts.  Instead of swapping mere rumors, PeX participants exchange *gossip records*, containing the address and peer ID of hosts in the network.  PeX gossip shares two noteworthy properties with high-school rumor-mongering:

1. Gossiped information may be outdated, or even flat-out untrue.
2. There are redundant exchanges.  Hosts, like students, do not know *a priori* whether their peers already have the information they are about to share.

The role of PeX is to provide a uniform sampling of peers as efficiently and securely as possible, given the above constraints.  To achieve this, the PeX protocol proceeds in synchronous *gossip rounds* between pairs of hosts, which are initiated by each peer at regular intervals.  During each gossip round, the initiator picks a peer from it's cache and connects to it.  The peers then send their respective cache contents to each other, and merge it with their own and retaining up to some maximum number of peers.

Gossip rounds are initiated at regular intervals by all peers.  It is not necessary for this interval to be identical for all peers.  Implementations SHOULD jitter the interval to reduce contention for network resources.

A gossip round unfolds in three steps.

1. **Peer Selection**.  The initiator selects a peer from its cache at random connects to it.
2. **Push-Pull**.  The peers exchange their respective views.
3. **View Merging**.  Each peer combines its current view the one it has received to form its new view.

These steps are examined in more detail in subsequent sections.

## Record Age

Multiple stages of the PeX protocol depend on a notion of a gossip record's _age_. To this end, gossip records contains an integer field called `Hop`, which is incremented when the record is received by a peer during a gossip round.  The value of this hop-counter is taken as the "age" of a record, as it increases over time.

During the push-pull phase, each node appends its own record, with `Hop=0`, to the view it transmits its counterparty.  This serves as a heartbeat mechanism, ensuring that "young" records are perpetually injected into the cluster by active nodes.  As we will see, "old" records are probabilistically culled, ensuring liveness.

## Peer Selection

The initiator randomly draws an entry from its cache and and opens a [PeX protocol stream](#stream-identifier) to the corresponding peer.  Implementations SHOULD repeat this process if the selected node proves unreachable but MUST NOT evict it from the cache at this stage.  Implementations SHOULD NOT initiate more than one gossip round at a time.

The randomized selection policy provides faster partition-healing than alternative, such as selecting the youngest or oldest neighbor.<sup>2</sup>

## Push-Pull

Each host sends a subset of its view to the other.  Let *c* be the maximum cache size (a value of `32` is RECOMMENDED). The subset is selected by taking the *c-P* "youngest" records, and appending the sender's own record.  For the avoidance of doubt, a peer MUST transmit its entire view if `len(view) <= P`, and MUST always append its own routing record to the stream.  The final item in the stream MUST have an age of `0`.

The pseudocode for the push-pull phase is as follows:
```
view.RandomShuffle()
view.MoveToTailOldest(P)
buffer = view.Head((c/2)-1)
buffer.append(MyRoutingRecord) 
Push(buffer)
```

It is important to note that the random shuffling and moving oldest entries to the tail is performed in-place. That is to say, the order of the elements in the view is permanently changed. This is crucial for other mechanisms during view merging (e.g. swapping).

## View Merging

View merging is the most delicate phase of the PeX protocol. It comprises five steps:

1. **Merge** the received and local views by joining them into a single list and
   removing duplicate entries. Local entries are put first, in front of the remote
   entries.
2. **Swap** the first `S` records in the merged view by removing them.  Recall that
   local entries were placed at the head of the list in the previous step, so removing
   these is tantamount to "swapping" `S` records with the remote peer.   In effect, 
   `S` is used to control priority given to the remote view entries over the records
   already known to a node.
3. **Protect-and-Decay:** the oldest `P` items are moved into a separate buffer. Then,
   `D` items are selected at random and discarded from the main buffer.
   The `P` oldest that have been set aside are effectively protected from eviction.
   This a crucial step in deriving PeX's strong partition-resistance properties.
4. **Evict** items from the merged list at random until the combined size of the main
   and protected buffers is less than or equal to `c`.  Then, append the the buffer
   protected buffer to the main buffer.
5. **Increase hops**: the age (hop counters) of the entries of the resulting view
   are increased by one.

The pseudocode for the previous four steps is the following:
```
remote = Pull()
view = view.append(remote)
view.RemoveHead(min(S, view.Size-c))
oldest = view.PopOldest(min(P, view.Size-c))
oldest.RemoveWithPropbability(D)
view.RemoveRandom(view.Size-(c-oldest.Size)))
view.append(oldest)
view.IncreaseAge()
```

Each step is described in detail below.

### Record Deduplication

**PeX** leverages the `Seq` counter in libp2p's `peer.Record` during view merging to deduplicate records and ensure the latest addressing information is preserved.  When merging views, hosts MUST evict records that:

1. Refer to themselves
2. Are duplicates

A record is considered a duplicate iff it has the same `PeerID` field as another record in the union of local and remote views.

When duplicates are encountered, peers MUST preserve records with the following priority:

1. A higher `Seq` number.
2. A higher `Hop` count.

This behavior makes it possible for hosts to update their network address when it changes.  To do so, the host begins issuing a new gossip record with the updated address information, and an incremented `Seq` number.

### Parameters

1. **S**(wapping): the number of items to be evicted from the head,
   after merging local and remote views. The local view is put in front of the
   remote one, so removing from the head, means prioritizing entries from the
   remote view. This parameter is used to reduce the randomness in the network.
   The higher the value, the more random the network will be.
2. **P**(rotection): the number of oldest entries to be protected from eviction.
   This parameter is used to tune the speed at which disconnected/partitioned 
   peer are removed from the network. The higher the value, the slower the speed. 
3. **D**(ecay): the probability of evicting entries in the "protected" buffer.
   The decay policy is applied by repeatedly drawing from a uniform distribution
   and comparing against the threshold `D`.  If a sample `d < D`, the youngest 
   record in the protected buffer is evicted.  This process repeates until `d >= D`
   or the protected buffer is empty.  In effect, the decay parameter `D` controls
   the speed at which stale records are purged from the network.  Higher values of
   `D` remove stale records more quickly, at the cost of partition-resistance.


# API

## Stream Identifier

**PeX** streams are identified by a dynamic protocol ID with the pattern:

```
/casm/<version>/pex/<ns>
```

The `<version>` path component MUST contain a valid [semantic version string](https://semver.org/spec/v2.0.0.html).  Hosts MUST reject connections from peers with incompatible versions.

The `<ns>` component is an arbitrary string that designates a namespace.  This namespace string MUST be consistent across different layers of the CASM protocol stack.  It must notably match the namespace used by the clustering layer.

## Gossip Record

The schema for gossip records is defined in `api/pex.capnp`.

```capnp
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
```

Views are transmitted as an LZ4 compressed stream of *un*packed `Gossip` structs, using standard Cap'n Proto framing.

# Known Issues

None (...so far!)

## Authors
- [@lthibault](https://github.com/lthibault)
- [@aratz-lasa](https://github.com/aratz-lasa) ★

★ Project Lead

## References
1. Jelasity, Márk, et al. "Gossip-based peer sampling." ACM Transactions on Computer Systems (TOCS) 25.3 (2007): 8-es.
1. Manterola-Lasa, Aratz, Diego Casado-Mansilla, and Diego López-de-Ipiña. "UnsServ: unstructured peer-to-peer library for deploying services in smart environments." 2020 5th International Conference on Smart and Sustainable Technologies (SpliTech). IEEE, 2020.
