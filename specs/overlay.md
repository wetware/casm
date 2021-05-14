# Overlay

> Nodes overlay maintenance

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

- [Overlay](#overlay)
    - [Table of Contents](#table-of-contents)
    - [Overview](#overview)
    - [Definitions](#definitions)
    - [Gossiping](#gossiping)
        - [Mechanism](#mechanism)
        - [Policies](#policies)
        - [Neighborhood maintenance](#neighborhood-maintenance)
    - [Libp2p](#libp2p)
        - [Connections](#connections)
        - [Streams](#streams)
        - [Peers](#peers)
    - [API](#api) 
    - [Issues](#issues)
    - [References](#references)

## Overview
[Overlay](https://github.com/wetware/casm/blob/005570a64b9cb311c5821a915d43f29317d72da9/pkg/net/net.go#L29) 
takes care of nodes overlay maintenance. In other words it is responsible for 
ensuring connectivity, so that all the nodes form a single connected network. 
Also, it provides an API for retrieving a random set of nodes from the network 
at any given time.

To do this, it uses a gossiping protocol that forms an unstructured peer-to-peer overlay. This protocol is based on [The Peer Sampling Service](https://dl.acm.org/doi/abs/10.5555/1045658.1045666).

## Definitions
- **Overlay**: a network that is layered on top of another network. The nodes within
  the overlay can be seen as connected to other nodes via logical paths or links,
  which correspond to a path in the underlying network. The way to visualize
  and understand the overlay is as a graph where vertices are nodes and edges are logical links.
  Note that if it is written capitalized *Overlay*, it refers to the protocol
  of the overlay maintenance, instead of to the overlay network.
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

Overlay implements gossiping for maintaining the overlay. Instead of spreading rumours it spreads nodes that are within the network.
Every t time units, each node selects a neighbour. Then, it
contacts with the neighbor, and they exchange their
current set of neighbors (also named "local views"). After the exchange, each node generates a
new set of neighbors, by merging the received neighbours with
the current ones. In order to implement this 50% of probability, a deterministic approach is used. 
The PeerIDs of the two gossiping nodes are XORed. If the most 
significant bit of the resulting bit is 0 or 1, the head or random 
policies are used, respectively.


This is how the Overlay protocol forms and maintains the overlay. 
Moreover, it also provides an API call for retrieving the current set of neighbors, and some piggybacked information about them. 

### Policies
Check [Definitions](#definitions) for understanding what a policy is 
and what are the three main alternatives.

There are two different policies gossiping must implement:
1. How to choose the neighbor with whom to gossip.
2. How to choose what neighbors to keep when merging the newly discovered 
   and current ones.

Overlay protocol uses a chooses the neighbor with whom to gossip randomly (`rand`). 
This provides a faster self-healing overlay than alternative policies 
such as choosing the youngest (`young`) or oldest (`old`) neighbor.

For choosing the new set of neighbors, Overlay implements a hybrid policy. 
50% of the times it selects the new neighbors randomly (`rand`). 
While the other 50% of the times, it selects the youngest (`young`) nodes.

For keeping track of the age of the neighbors, the nodes are represented 
through Libp2p PeerRecord structs. [Below](#peerrecord) it is further explained.

### Neighborhood maintenance
What does Overlay do after updating the neighbors due to a gossiping exchange?

For each newly discovered neighbor it opens a [connection](https://docs.libp2p.io/reference/glossary/#connection). 
To the contrary, if a neighbor is unselected after the gossiping, 
the connection is closed.

Note that the connections are bidirectional. So, when a node opens a connection
with a neighbor, the neighbor adds the node to its neighborhood too. 
This way, both nodes have each other as neighbors. In case the 
neighbor already has too many opened connections, it prioritizes 
the newly incoming connection. So, it is forced to evict a current neighbor
and close the corresponding connection. 
In order to choose what connection to close, it follows the same policy 
as for choosing new neighbors. But, instead of selecting the youngest nodes
50% of the times, it chooses the oldest. This way, the youngest nodes are 
kept in the neighborhood. 

### Nodes age

TODO: explain hop counter and the heartbeat mechanism

## Libp2p
Overlay makes use of Libp2p for opening/handling connections, 
multiplexing over these connections and managing peers identities.

### Connections
At all times, every node is connected to a set of neighbors. For opening
connections to the neighbors and maintaining them opened, Overlay initializes 
a [Libp2p Host](https://github.com/libp2p/go-libp2p-core/blob/525a0b13017263bde889a3295fa2e4212d7af8c5/host/host.go#L25) 
and makes use of [Connect()](https://github.com/libp2p/go-libp2p-core/blob/525a0b13017263bde889a3295fa2e4212d7af8c5/host/host.go#L46) and 
[ConnManager()](https://github.com/libp2p/go-libp2p-core/blob/525a0b13017263bde889a3295fa2e4212d7af8c5/host/host.go#L72) APIs. 

### Streams
When a gossiping exchange is started, it needs to communicate with a neighbor 
through a connection. However, this communication should not interfere 
with other higher layer protocols. Therefore, the node needs to multiplex 
the connection. To do this, it makes use of Libp2p [multiplexer](http://docs.libp2p.io/concepts/stream-multiplexing/) 
and opens a [stream](https://docs.libp2p.io/reference/glossary/#multiplexing) 
for performing the gossiping exchange.

TODO: specify the stream IDs

### Peers
Overlay makes use of Libp2p to manage peer identities. 
It uses the [PeerRecord](https://github.com/libp2p/go-libp2p-core/blob/master/peer/record.go) 
data structure to represent the peers on the network. PeerRecord has three 
fields: [PeerID](http://docs.libp2p.io.ipns.localhost:8080/reference/glossary/#peerid) 
that is unique identifier, an array of [multiaddresses](http://docs.libp2p.io.ipns.localhost:8080/reference/glossary/#multiaddr) 
for routing to the node, and `Seq` that represents the age of the node. This 
last field is necessary to implement gossiping `young` and `old` policies.

## API
Overlay API provides four calls. The interface in Golang code is shown below 
(it is a simplified version, the real code has extra Golang-specific parameters):
```go
type Overlay interface{
	
    New(h host.Host, opt ...Option) (*Overlay, error)	

    Close() error
	
    Join(d discovery.Discoverer, opt ...discovery.Option) error

    Stat() Status
}
```

- `New()`: initializes the node and all its internal mechanisms. 
  It receives a Libp2p Host as a parameter together with some configurable options.
   
- `Close()`: leaves the overlay gracefully. Stops all the internal mechanisms and 
  drops all the opened connections. This should be called before leaving the overlay.

- `Join()`: idempotent mechanism for joining two graphs of the overlay. 
  Receives as parameters a Libp2p Discoverer, which provides peers from 
  the graph to join. This is intended to use for bootstrapping but also 
  for repairing and avoiding partitions. 

- `Stat()`: returns the current state of the overlay. The state is represented by 
  a `Status` data structure that contains the IDs of the neighbors. 
  This call is expected to be extended with a richer set of descriptors about the state.

## Issues

## References
- [The Peer Sampling Service:
  Experimental Evaluation of Unstructured
  Gossip-Based Implementations](https://dl.acm.org/doi/abs/10.5555/1045658.1045666)
  
- [Libp2p](https://libp2p.io) 