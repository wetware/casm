# pex

> Peer-Exchange algorithm used as peer discovery service

| Lifecycle Stage | Maturity       | Status | Latest Revision |
|-----------------|----------------|--------|-----------------|
| 1A              | Working Draft  | Active | r1, 2021-05-11  |

Authors: [@aratz-lasa], [@lthibault]

Interest Group: None

[@aratz-lasa]: https://github.com/aratz-lasa
[@lthibault]: https://github.com/lthibault

See Libp2p's [lifecycle document][lifecycle-spec] for context about maturity level
and spec status.

[lifecycle-spec]: https://github.com/libp2p/specs/blob/master/00-framework-01-spec-lifecycle.md

## Table of Contents

- [pex](#pex)
    - [Table of Contents](#table-of-contents)
    - [Overview](#overview)
    - [Definitions](#definitions)
    - [Gossiping](#gossiping)
        - [Mechanism](#mechanism)
        - [Policies](#policies)
        - [Nodes age](#nodes-age)
    - [Libp2p](#libp2p)
        - [Host and streams](#host-and-streams)
        - [Peers](#peers)
    - [API](#api)
    - [Issues](#issues)
    - [References](#references)

## Overview
**pex** stands for *Peer-Exchange* and 
takes care of maintaining a peer-to-peer unstructured overlay. In other words it is responsible for
ensuring connectivity, so that all the nodes form a single connected random network.
Also, it provides a [Discovery](https://github.com/libp2p/go-libp2p-core/blob/master/discovery/discovery.go) API 
for providing a random set of nodes from the network at any given time. This is mainly used by other services 
that need to discover new peers for staying connected in the network.

To do this, it uses a gossiping protocol that forms an unstructured peer-to-peer overlay. 
This protocol is based on [The Peer Sampling Service](https://dl.acm.org/doi/abs/10.5555/1045658.1045666).

## Definitions
- **Overlay**: a network that is layered on top of another network. The nodes within
  the overlay can be seen as connected to other nodes via logical paths or links,
  which correspond to a path in the underlying network. The way to visualize
  and understand the overlay is as a graph where vertices are nodes and edges are logical links.
- **Peer**: is a synonym of node.
- **Neighbor/neighborhood**: the nodes to which are directly connected via a logical link,
  without any intermediate hops in the overlay. So, the neighborhood is the set
  of neighbors.
- **Gossiping**: mechanism for spreading information within an overlay. In this
  particular case, it spreads information about the nodes of the overlay. So
  it's more accurate to refer to it as a mechanism for maintaining the overlay,
  rather than for spreading information.
- **Policy**: in a gossiping context, it refers to the selection strategy used
  for choosing a node from a group of nodes. In gossiping there are three main
  policies: `rand`, `young` and `old`. `rand` chooses a node randomly. `young`
  and `old` choose the youngest and oldest nodes, respectively.
  The age of a node is based on a heartbeat mechanism. So, the more recent the
  heartbeat is received, the younger the sender node will be.


## Gossiping

### Mechanism
Gossiping can be well understood when
comparing it with how humans spread rumours. Imagine
a high-school where students talk to each other in the hall
whenever they have a break. Each student encounters other
randomly, and it shares the rumour. If a new rumour is first
told in the morning, by the end of the day, almost every student will have heard about it. Notice that when a gossip is
shared, they do not know if the other student already knows
about it. So, there will be some redundant exchanges.

**pex** implements gossiping for maintaining the overlay. Instead of spreading rumours it spreads nodes that are within the network.
Every *t* time units, each node selects a neighbour. Then, it
contacts with the neighbor, and they exchange their
current set of neighbors (also named "views"). After the exchange, each node generates a
new set of neighbors, by merging the received neighbours with
the current ones. In order to implement this 50% of probability, a deterministic approach is used.
The PeerIDs of the two gossiping nodes are compared. If the sender's ID is higher or lower than 
receiver's, the head or random policies are used, respectively.


This is how the **pex** protocol forms and maintains the overlay.
Moreover, it also provides an API call for retrieving the current set of neighbors, and some piggybacked information about them.

### Policies
Check [Definitions](#definitions) for understanding what a policy is
and what are the three main alternatives.

There are two different policies gossiping must implement:
1. How to choose the neighbor with whom to gossip.
2. How to choose what neighbors to keep when merging the newly discovered
   and current ones.

**pex** protocol uses a chooses the neighbor with whom to gossip randomly (`rand`).
This provides a faster self-healing overlay than alternative policies
such as choosing the youngest (`young`) or oldest (`old`) neighbor.

For choosing the new set of neighbors, **pex** implements a hybrid policy.
50% of the times it selects the new neighbors randomly (`rand`).
While the other 50% of the times, it selects the youngest (`young`) nodes.

For keeping track of the age of the neighbors, the nodes are represented
through Libp2p `PeerRecord` structs. [Below](#peerrecord) it is further explained.

### Nodes age
In order to implement the youngest or oldest strategies it's necessary to
keep the ages of the nodes in the network. Therefore, the peers that are kept
in the local view have a hop counter. This is a counter that represents the age of
the peers. In this way, when one peer sends its local view to another, the receiver 
increases the hop counter of those peers by one. That is, every time a peer
information is passed from one node to another, its counter is increased. Thus, 
the higher the value, the older the information for that peer.

When a node sends its local view to another, the sender adds itself to the local 
view with a Hop Counter of zero. This is a heartbeat mechanism. When adding
itself, it will be the youngest peer of the local view that receives the other node.

## Libp2p
Overlay makes use of Libp2p for opening connections,
multiplexing over these connections and managing peers identities.

### Host and streams
For opening connections to the neighbors and exchange the views, **pex** initializes
a [Libp2p Host](https://github.com/libp2p/go-libp2p-core/blob/525a0b13017263bde889a3295fa2e4212d7af8c5/host/host.go#L25).
When a gossiping exchange is started, it needs to communicate with a neighbor
through a connection. However, this communication should not interfere
with other higher layer protocols. Therefore, the node needs to multiplex
the connection. To do this, it makes use of Libp2p [multiplexer](http://docs.libp2p.io/concepts/stream-multiplexing/)
and opens a [stream](https://docs.libp2p.io/reference/glossary/#multiplexing)
for performing the gossiping exchange.

TODO: specify the stream IDs

### Peers
**pex** makes use of Libp2p to manage peer identities.
It uses the [PeerRecord](https://github.com/libp2p/go-libp2p-core/blob/master/peer/record.go)
data structure to represent the peers on the network. `PeerRecord` has three
fields: [PeerID](http://docs.libp2p.io.ipns.localhost:8080/reference/glossary/#peerid)
that is unique identifier, an array of [multiaddresses](http://docs.libp2p.io.ipns.localhost:8080/reference/glossary/#multiaddr)
for routing to the node, and `Seq` that represents the age of the node. This
last field is necessary to implement gossiping `young` and `old` policies.

## API
**pex** API provides four calls. The interface in Golang code is shown below
(it is a simplified version, the real code has extra Golang-specific parameters):
```go
type Overlay interface{
	
    New(h host.Host, opt ...Option) (*Overlay, error)	

    Close() error
	
    Join(d discovery.Discoverer, opt ...discovery.Option) error

    FindPeers() Peers
}
```

- `New()`: initializes the node and all its internal mechanisms.
  It receives a Libp2p Host as a parameter together with some configurable options.

- `Close()`: closes the peer exchange service. Stops all the internal mechanisms and
  drops all the opened connections.

- `Join()`: idempotent mechanism for adding new nodes to the local view.
  Receives as parameters a Libp2p Discoverer, which provides peers to add. This is intended to use for bootstrapping but also
  for repairing and avoiding partitions.

- `FindPeers()`: provides peers from the overlay that pex maintains. In other words,
  it discovers peers providing a service.

## Issues

## References
- [The Peer Sampling Service:
  Experimental Evaluation of Unstructured
  Gossip-Based Implementations](https://dl.acm.org/doi/abs/10.5555/1045658.1045666)

- [Libp2p](https://libp2p.io) 