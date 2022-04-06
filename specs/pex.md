# Peer Exchange (PeX)

Gossip-based sampling service for robust connectivity.

| Lifecycle Stage | Maturity                 | Status | Latest Revision |
|-----------------|--------------------------|--------|-----------------|
| 2A              | Candidate Recommendation | Active | r1, 2021-08-26  |

Authors: [@aratz-lasa], [@lthibault]

[@aratz-lasa]: https://github.com/aratz-lasa
[@lthibault]: https://github.com/lthibault

See libp2p's [lifecycle document][lifecycle-spec] for context about maturity level
and spec status.

[lifecycle-spec]: https://github.com/libp2p/specs/blob/master/00-framework-01-spec-lifecycle.md

## Table of Contents

- [Peer Exchange (PeX)](#peer-exchange-pex)
  - [Table of Contents](#table-of-contents)
  - [Motivation](#motivation)
  - [Protocol Specification](#protocol-specification)
    - [Overview](#overview)
    - [Conventions](#conventions)
    - [Definitions](#definitions)
    - [Gossiping](#gossiping)
      - [Mechanism](#mechanism)
      - [Peer Selection](#peer-selection)
      - [Push-Pull](#push-pull)
      - [View Merging](#view-merging)
      - [View Merging Policies](#view-merging-policies)
      - [Node Age](#node-age)
      - [Record Sequence](#record-sequence)
    - [API](#api)
      - [Stream Identifier](#stream-identifier)
      - [Peer Exchange](#peer-exchange)
      - [Gossip Record](#gossip-record)
      - [Wire Format](#wire-format)
  - [Known Issues](#known-issues)
  - [Core Team](#core-team)
  - [References](#references)

## Motivation

Once a host has successfully joined a cluster, it ceases to rely on the bootstrap service for peer-discovery, and instead relies on the cluster itself to discover new peers.  This process of "ambient peer discovery" is provided as an unstructured service called PeX (short for peer exchange).  In contrast to DHT and PubSub-based methods, PeX can be used to repair cluster partitions, or to rejoin a cluster after having been disconnected.

The motivation for PeX is reliability.  CASM is designed with peer-to-peer, edge, mobile and IoT applications in mind.  Nodes are expected to change their network addresses and experience transient network outages frequently.  Such churn presents a challenge for software designers, who must ensure that nodes recover from partitioned states quickly, ideally without resorting to a centralized service.  Instead, hosts should maintain a “recovery cache” — a small, continuously-updated list of peers for partition repair and post hoc bootstrapping — and fall back on the bootstrap layer as a last resort.

The ideal recovery cache would exhibit the following properties:

1. **Persistence.**  Items should remain in the cache until they have been explicitly evicted or replaced with suitable alternatives.  Avoid explicit timeouts and ttls, which can make partitions permanent.

2. **Partition-Resistance.**  The cache should remain valid during a partiton and the speed at which peers in one partiton evict peers in othe another should be controllable.

3. **Liveness.**  Changes to host addresses should quickly propagate through the network.  Dead peers should eventually disappear from cache.  New records should overwrite old records.

4. **Unbiased Sampling.**  Peers should be equally likely to appear in a given cache instance.  This is essential to prevent the formation of partitions and to avoid overloading peers when partitions merge.

5. **Scalability.**  The global system should stabilize in better-than-linear time.

Crucially, the PubSub router and the Kademlia DHT fail to satisfy one or more of these requirements.

## Protocol Specification

PeX provides a random sample of networked peers to applications.  It adheres to libp2p's [Discovery](https://github.com/libp2p/go-libp2p-core/blob/master/discovery/discovery.go) API, allowing it to seamlessly integrate with existing application.  In particular, it is suitable for use with PubSub.

PeX continuously updates a cache of random peers by gossipping with the peers currently in the cache.  The result is an unstructured overlay network, similar to the one described in *[Gossip-Based Peer Sampling](https://dl.acm.org/doi/abs/10.1145/1275517.1275520)*.

### Conventions

>The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT", "SHOULD", "SHOULD NOT", "RECOMMENDED",  "MAY", and "OPTIONAL" in this document are to be interpreted as described in [RFC 2119](https://datatracker.ietf.org/doc/html/rfc2119) and [RFC 2119](https://datatracker.ietf.org/doc/html/rfc8174).

### Definitions
- **Overlay**: a network that is layered on top of another network. The nodes within the overlay can be seen as connected to other nodes via logical paths or links, which correspond to a path in the underlying network.
- **Peer**: a physical host in the overlay.  Synonymous with node.
- **Neighbor/Neighborhod**: adjacent nodes in the overlay.  These nodes are directly connected via a libp2p transport connection, without any intermediate hops in the overlay.  A node's neighborhood is the set of its neighbors.
- **View**:  A node's view of the cluster is a set of [`GossipRecord`](#gossip-record) instances, which contains addressing information.  Importantly, the view is assumed to be small with respect to the overall size of the cluster.  Throughout the specification, the maximum view size for each node is defined by the cluster-wide constant `c`.
- **Gossiping**: a broadcast mechanism in which peers relay messages to their neighbors.
- **Gossip Round**:  a single exchange of gossip with a neighbor.
- **Merge Policy**: in the context of a gossip round, this refers to strategy for selecting a new view from the union of one's local view and a view received from a peer.
- **Eviction**:  the process of removing an entry from the local view of a peer.  The term "eviction" may also be applied in the context of an entire cluster to refer to the process by which an accumulation of evictions by individual nodes results in the record being purged from a given partition.

### Protocol Description

Gossiping can be well understood by analogy.  Imagine a high-school where students talk to each other in the hall whenever they have a break. Each student encounters other randomly, and shares the rumour. If a new rumour is first told in the morning, by the end of the day, almost every student will have heard about it.

Instead of spreading mere rumors, PeX spreads records of nodes that are within the network.  Nevertheless, these records share two noteworthy properties with rumors:

1. They may be untrue.  The information may be outdated or false.
2. There are redundant exchanges.  Nodes, like students, do not know *a priori* whether their peers already have the information they are about to share.

In PeX, gossip occurs in synchronous *gossip rounds* between pairs of nodes.  Gossip rounds are initiated periodically by each peer, every *t* seconds.  The value of *t* SHOULD be jittered to avoid message storms.

Gossip rounds comprise three steps:

1. **Peer Selection**.  A node randomly selects one peer from its current view according to a _view selection policy_.  It MAY repeat this process if the selected node proves unreachable.
2. **Push-Pull**.  The peers exchange their respective views.
3. **View Merging**.  Each peer combines its current view with the freshly received view and selects duplicate entries.  If the length of a combined view exceeds _c_, the affected node selects a subset of records to form the final view according to a _merge policy_.

We examine each phase of the protocol in detail below.

<!-- contacts with the neighbor, and they exchange their
current set of neighbors (also named "views"). After the exchange, each node generates a
new set of neighbors, by merging the received neighbours with
the current ones. In order to implement this 50% of probability, a deterministic approach is used.
The PeerIDs of the two gossiping nodes are compared. If the sender's ID is higher or lower than 
receiver's, the head or random policies are used, respectively.


This is how the **pex** protocol forms and maintains the overlay.
Moreover, it also provides an API call for retrieving the current set of neighbors, and some piggybacked information about them.
 -->
 
#### Peer Selection

PeX chooses the neighbor with whom to gossip randomly (`rand`).
This provides a faster self-healing overlay than alternative policies
such as choosing the youngest (`young`) or oldest (`old`) neighbor.

#### Push-Pull

After the peer selection, the selected and selector nodes connect with each
other and send their views. However, they generally do not send the entire view, but instead
select the *c-P* "youngest" records.  If, however, `len(view) <= P`, the peer  MUST transmit
its entire view.  We define the notion of record age and provide additional details regarding
the *P* parameter below.

Finally, each peer transmits a record containing its own routing information.
<!-- 
At most, they send half of the maximum view size. The transmitted records are selected randomly,
ignoring the oldest `P` nodes. However, in case there aren't enough nodes, 
the oldest nodes are also sent. Finally, a descriptor of the sender is appended
to the tail, before sending it. -->

The pseudocode for the push-pull phase is as follows:
```
view.RandomShuffle()
view.MoveToTailOldest(P)
buffer = view.Head((c/2)-1)
buffer.append(MyDescriptor) 
Push(buffer)
```

It is important to note that the random shuffling and moving oldest entries
to the tail is done in-place. That is to say, the order of the elements in
the view is permanently changed. This is crucial for other mechanisms
during view merging (e.g. Swapping).

#### View Merging

View merging is the most complex and delicate phase of the PeX protocol. It comprises
five steps:

1. **Merge** the received and local views by joining them into a single list and
   removing duplicate entries. Local entries are put first, in front of the remote
   entries.
2. **Swap** the first `S` records in the merged view by removing them.  Recall that
   local entries were placed at the head of the list in the previous step, so removing
   these is tantamount to "swapping" `S` records with the remote peer.   In effect, 
   `S` is used to control priority given to the remote view entries over the records
   already known to a node.
3. **Protect-and-Decay:** the oldest `P` items are moved into a separate buffer. Then,
   `D` items are selected at random and discarded from removed from the main buffer.
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

#### View Merging Policies
Pex makes use of three parameters to tune the policies for merging the view.

- **S**(wapping): the number of items to be evicted from the head,
  after merging local and remote views. The local view is put in front of the
  remote one, so removing from the head, means prioritizing entries from the
  remote view. This parameter is used to reduce the randomness in the network.
  The higher the value, the more random the network will be.
- **P**(rotection): the number of oldest entries to be protected from eviction.
  This parameter is used to tune the speed at which disconnected/partitioned 
  peer are removed from the network. The higher the value the slower the speed. 
- **D**(ecay): the probability of evicting the protected oldest entries. 
  `D` probability is applied by doing `rand.RandomFloat64<D`. If it results to
  `True`, the youngest (among oldest) is evicted and the probability is applied
  again. But if it results to `False`, no entry is evicted, and consequent
  evictions applying `D` stop. This parameter is used to tune the speed at which
  disconnected/partitioned peer are removed from the network. The higher the 
  probability the higher the speed.

#### Node Age
In order to implement the ploicies, it is necessary to track the age of each gossip 
record contained in a node's view. To this end, `GossipRecord` contains a
`Hop` field, _i.e._ a counter that is incremented by a peer when it receives 
the record in the course of a gossip round.  The hop counter represents 
the "age" of a node insofar as the number of gossip rounds 
(and therefore network hops) of a record are a function of *t*. After every 
gossiping exchange, the ages of the nodes stored in the resulting view are increased. 

When a node sends its local view to another, the sender adds itself to the local 
view with a Hop Counter of zero. This is a heartbeat mechanism. When adding
itself, it will be the youngest peer of the local view that receives the other node.

#### Record Sequence

**PeX** leverages the `Seq` counter in libp2p's `peer.Record` during view merging to deduplicate records and ensure the latest addressing information is preserved.  When merging views, peers should evict records that:

1. Refer to themselves
2. Are duplicates

A `GossipRecord` is considered a duplicate iff it has the same `PeerID` field as another record in the union of local and remote views.  In order of priroity, preserve with:

1. A higher `Seq` number.
2. A higher `Hop` count.

This ensures hosts can update their network address when it changes.
### API

#### Stream Identifier

**PeX** streams are identified by a dynamic protocol ID with the pattern:

```
/casm/pex/<version>/<ns>
```

The `<version>` path component MUST contain a valid [semantic version string](https://semver.org/spec/v2.0.0.html).  Hosts MUST reject connections from peers with incompatible versions.

The `<ns>` component is an arbitrary string that designates a namespace.  This namespace string MUST be consistent across different layers of the CASM protocol stack.  It must notably match the namespace used by the clustering layer.

#### Peer Exchange

The **PeX** API is defined by four methods.  These are shown below in pseudocode using Go-like syntax.

```go
type PeerExchange interface {
  // New peer exchange.  The cluster is identified by 'ns'.
  New(ctx context.Context, h host.Host, ns string, opt ...Option) (*PeerExchange, error)	
	
  // Close stops all gossip activity and invalidates the peer exchange.
  Close() error
	
  // FindPeers discovers peers providing a service
  Advertise(ctx context.Context, ns string, opts ...Option) (time.Duration, error)

// Advertise advertises a service
  FindPeers(ctx context.Context, ns string, opts ...Option) (<-chan peer.AddrInfo, error)
}
```

Note that `Advertise` and `FindPeers` implement the 
[Discovery interface](https://github.com/libp2p/go-libp2p-discovery) of Libp2p.  

#### Gossip Record

Gossip records are the messages that are exchanged during gossip rounds.  Each gossip record is signed by the peer that initially emits it.  Thus, each record's signature matches the `PeerID` field in its `peer.PeerRecord`.

```go
type GossipRecord struct {
	Hop uint64
	peer.PeerRecord
	*record.Envelope
}
```

When receiving a remote view, implementations MUST validate the following:

1. The last record MUST have `Hop == 0`.
2. All other records MUST have `Hop > 0`.
3. Records MUST be signed the peer `GossipRecord.PeerID`.
4. The last record sent MUST be signed by the sender.

Future work will focus on implementing a peer scoring system similar to GossipSub, which will punish peers who send invalid views by refusing to gossip with them.


#### Wire Format

**PeX** uses a simple binary encoding to transmit data during gossip rounds.  Fixed-size values such as `Hop` are encoded using _unsigned_ 64-bit varints, and variable-length content is length-prefixed using _signed_ varints.

Thus, the wire format for a single `GosspRecord` is the following:

```
+---------------+--------------+--------------------------------+
| Seq (uvarint) | len (varint) | signed record.Envelope (bytes) |
+---------------+--------------+--------------------------------+
```

The default encoding for `record.Envelope` is used (protocol buffers).

Views are transmitted as a sequence of length-prefixed `GossipRecord` messages.  Receivers can check that they have received the expected number of records for a given view by validating that the final record refers to the sender and has a hop count of zero.  As noted above, all records MUST be signed by the peers they reference rather than the immediate sender.

The wire format for a view is shown below.  The `...` indicates that the pattern can repeat an arbitrary number of times.

```
+--------------+----------------------+-----+
| len (varint) | GossipRecord (bytes) | ... |
+--------------+----------------------+-----+
```

To protect against DoS attacks, implementations SHOULD limit the length of encoded views received by neighbors to some fixed multiple of the maximum value size.  A value of 1024 (1 Kb) is RECOMMENDED.

## Known Issues

None (...so far!)

## Alternative designs
TODO:

## Core Team

- [@lthibault](https://github.com/lthibault)
- [@aratz-lasa](https://github.com/aratz-lasa) ★

★ Project Lead

## References
1. 	Jelasity, Márk, et al. "Gossip-based peer sampling." ACM Transactions on Computer Systems (TOCS) 25.3 (2007): 8-es.
