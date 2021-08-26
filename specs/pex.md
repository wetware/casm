# Peer Exchange (PeX)

Gossip-based sampling service for robust connectivity.

| Lifecycle Stage | Maturity       | Status | Latest Revision |
|-----------------|----------------|--------|-----------------|
| 1A              | Working Draft  | Active | r1, 2021-05-11  |

Authors: [@aratz-lasa], [@lthibault]

[@aratz-lasa]: https://github.com/aratz-lasa
[@lthibault]: https://github.com/lthibault

See Libp2p's [lifecycle document][lifecycle-spec] for context about maturity level
and spec status.

[lifecycle-spec]: https://github.com/libp2p/specs/blob/master/00-framework-01-spec-lifecycle.md

## Table of Contents

- [Peer Exchange (PeX)](#peer-exchange-pex)
  - [Table of Contents](#table-of-contents)
  - [Motivation](#motivation)
  - [Protocol Specification](#protocol-specification)
    - [Overview](#overview)
    - [Definitions](#definitions)
    - [Gossiping](#gossiping)
      - [Mechanism](#mechanism)
      - [Peer Selection](#peer-selection)
      - [Merge Policy](#merge-policy)
      - [Node Age](#node-age)
    - [Record Sequence](#record-sequence)
    - [API](#api)
      - [Stream Identifier](#stream-identifier)
      - [Peer Exchange](#peer-exchange)
      - [Gossip Record](#gossip-record)
  - [Known Issues](#known-issues)
    - [Core Team](#core-team)
  - [References](#references)

## Motivation

Once a host has successfully joined a cluster, it ceases to rely on the bootstrap service for peer-discovery, and instead relies on the cluster itself to discover new peers.  This process of "ambient peer discovery" is provided as an unstructured service called PeX (short for peer exchange).  In contrast to DHT and PubSub-based methods, PeX can be used to repair cluster partitions, or to rejoin a cluster after having been disconnected.

The motivation for PeX is reliability.  CASM is designed with peer-to-peer, edge, mobile and IoT applications in mind.  Nodes are expected to change their network addresses and experience transient network outages frequently.  Such churn presents a challenge for software designers, who must ensure that nodes recover from partitioned states quickly, ideally without resorting to a centralized service.  Instead, hosts should maintain a “recovery cache” — a small, continuously-updated list of peers for partition repair and post hoc bootstrapping — and fall back on the bootstrap layer as a last resort.

The ideal recovery cache would exhibit the following properties:

1. **Persistence.**  Items should remain in the cache until they have been explicitly evicted or replaced with suitable alternatives.  Avoid explicit timeouts and ttls, which can make partitions permanent.

2. **Partition-Resistance.**  The cache should remain valid during a partiton and peers in one partiton should be slow to evict peers in othe another.

3. **Liveness.**  Changes to host addresses should quickly propagate through the network.  Dead peers should eventually disappear from cache.  New records should overwrite old records.

4. **Unbiased Sampling.**  Peers should be equally likely to appear in a given cache instance.  This is essential to prevent the formation of partitions and to avoid overloading peers when partitions merge.

5. **Resource Efficiency.**  The cache algorithm must use CPU,  RAM, disk storage and network bandwidth sparingly.  Resource use should also be bounded.
Scalability.  The global system should stabilize in better-than-linear time.

6. **Resource Efficiency.**  The cache algorithm must use CPU,  RAM, disk storage and network bandwidth sparingly.  Resource use should also be bounded.

7. **Scalability.**  The global system should stabilize in better-than-linear time.

Crucially, the PubSub router and the Kademlia DHT fail to satisfy one or more of these requirements.

## Protocol Specification

### Overview
**pex** stands for *Peer-Exchange* and 
takes care of maintaining a peer-to-peer unstructured overlay. In other words it is responsible for
ensuring connectivity, so that all the nodes form a single connected random network.
Also, it provides a [Discovery](https://github.com/libp2p/go-libp2p-core/blob/master/discovery/discovery.go) API 
for providing a random set of nodes from the network at any given time. This is mainly used by other services 
that need to discover new peers for staying connected in the network.

To do this, it uses a gossiping protocol that forms an unstructured peer-to-peer overlay. 
This protocol is based on [The Peer Sampling Service](https://dl.acm.org/doi/abs/10.5555/1045658.1045666).

### Definitions
- **Overlay**: a network that is layered on top of another network. The nodes within the overlay can be seen as connected to other nodes via logical paths or links, which correspond to a path in the underlying network.
- **Peer**: a physical host in the overlay.  Synonymous with node.
- **Neighbor/Neighborhod**: adjacent nodes in the overlay.  These nodes are directly connected via a libp2p transport connection, without any intermediate hops in the overlay.  A node's neighborhood is the set of its neighbors.
- **View**:  A node's view of the cluster is a set of [`GossipRecord`](#gossip-record) instances, which contains a record for each neighbor.
- **Gossiping**: a broadcast mechanism whereby each peer transmits each message to its neighbors.
- **Gossip Round**:  a single exchange of gossip with a neighbor.
- **Merge Policy**: in the context of a gossip round, this refers to strategy for selecting a new view from the union of one's local view and a view received from a peer.  PeX employs a hybrid of two strategies:  (1) `young` selects the *n* most recent records and (2) `rand` selects *n* records at random.


### Gossiping

#### Mechanism

Gossiping can be well understood by analogy.  Imagine
a high-school where students talk to each other in the hall
whenever they have a break. Each student encounters other
randomly, and shares the rumour. If a new rumour is first
told in the morning, by the end of the day, almost every student will have heard about it. Notice that when gossiping, students do not know whether or not the people they are talking to already know the rumor.  So, there will be some redundant exchanges.

**PeX** implements a peer-sampling service using a gossiping strategy.  Instead of spreading rumours it spreads records of nodes that are within the network.  This gossip takes place in *gossip rounds*.  Each node maintains a timer and initiates a gossip round every *t* seconds.  The value of *t* is jittered to avoid message storms.

Gossip rounds comprise three steps:

1. **Peer Selection**.  The node randomly selects one peer from its current view according to some policy.  It may repeat this process if the selected node is unreachable.
2. **Push-Pull**.  The peers exchange their respective views.
3. **View Merging**.  The peers each combine their current view with the one received by its counterpart, as per merge policy.  A subset of these peers are retained to form the new view.


contacts with the neighbor, and they exchange their
current set of neighbors (also named "views"). After the exchange, each node generates a
new set of neighbors, by merging the received neighbours with
the current ones. In order to implement this 50% of probability, a deterministic approach is used.
The PeerIDs of the two gossiping nodes are compared. If the sender's ID is higher or lower than 
receiver's, the head or random policies are used, respectively.


This is how the **pex** protocol forms and maintains the overlay.
Moreover, it also provides an API call for retrieving the current set of neighbors, and some piggybacked information about them.

#### Peer Selection

**PeX** uses a chooses the neighbor with whom to gossip randomly (`rand`).
This provides a faster self-healing overlay than alternative policies
such as choosing the youngest (`young`) or oldest (`old`) neighbor.

#### Merge Policy

**PeX** implements a hybrid policy for merging its view with its neighbor's during a gossip round.  PeX implements a decision criteria using the XOR of the last 8 bytes of both nodes' peer IDs.  If `xor(pid_a, pid_b) > 2^32-1`, it selects a node randomly (`rand` policy).  Else, it selects the youngest node (`young` policy; alternatively refered to as `tail` in source code).  This assigns equal probabilities to both policies.

The `young` policy results in a faster stabilization of the cluster, which is valuable for scalability.  However, partitions will tend to "forget" about each other as older nodes are aggressively purged.

The `rand` policy is slower to converge (linear time), but results in partitions with "memory".  Unreachable nodes will be retained for a significantly longer period of time.

The "age" of a node is tracked in [`GossipRecord`](#gossip-record), and discussed [below](#node-age).

For a complete review of merge policies and their effect on cluster performance, refer to [Jelasity *et al.*](#references).

#### Node Age
In order to implement the `young` policy, it is necessary to track the age of each gossip record contained in a node's view.  To this end, `GossipRecord` contains a `Hop` field, _i.e._ a counter that is incremented by a peer when it receives the record in the course of a gossip round.  The hop counter represents the "age" of a node insofar as the number of gossip rounds (and therefore network hops) of a record are a function of *t*.

It is also useful to visualize `Hop` as a measure of diffusion.  Senders do not increase the hop counter of the records they distribute; only receivers do so.  As such, a higher hop indicates the record has travelled a greater distance through the overlay.

When a node sends its local view to another, the sender adds itself to the local 
view with a Hop Counter of zero. This is a heartbeat mechanism. When adding
itself, it will be the youngest peer of the local view that receives the other node.

### Record Sequence

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

where `<version>` is a semantic version number and `<ns>` is the cluster's namespace string.

#### Peer Exchange

The **PeX** API is defined by four methods.  These are shown below in pseudocode using Go-likesyntax.

```go
type PeerExchange interface{
  // New peer exchange.  The cluster is identified by 'ns'.
  New(h host.Host, ns string, opt ...Option) (*PeerExchange, error)	
	
  // Close stops all gossip activity and invalidates the peer exchange.
  Close() error
	
  // Join the gossiping overlay, using an existing node as a bootstrap
  // peer.
  Join(ctx context.Context, boot peer.AddrInfo) error

  // View returns the gossip records currently contained in the overlay.
  View() []GossipRecord
}
```

#### Gossip Record

Gossip records are the messages that are exchanged during gossip rounds.  Each gossip record is signed by the peer that initially emits it.  Thus, each record's signature matches the `PeerID` field in its `peer.PeerRecord`.

```go
type GossipRecord struct {
	Hop uint64
	peer.PeerRecord
	*record.Envelope
}
```

When receiving a remote view, implementations should validate the following:

1. The last record MUST have `Hop == 0`.
2. All other records MUST have `Hop > 0`.
3. The last record MUST be signed by the neighbor that sent it.

Future work will focus on implementing a peer scoring system similar to GossipSub, which will punish peers who send invalid views by refusing to gossip with them.

## Known Issues

None (...so far!)

### Core Team

- [@lthibault](https://github.com/lthibault)
- [@aratz-lasa](https://github.com/aratz-lasa) ★

★ Project Lead

## References
1. 	M. Jelasity, R. Guerraoui, A.-M. Kermarrec, and M. van Steen, “The Peer Sampling Service: Experimental Evaluation of Unstructured Gossip-Based Implementations,” in Middleware 2004, vol. 3231, H.-A. Jacobsen, Ed. Berlin, Heidelberg: Springer Berlin Heidelberg, 2004, pp. 79–98. doi: 10.1007/978-3-540-30229-2_5. https://dl.acm.org/doi/abs/10.5555/1045658.1045666.
