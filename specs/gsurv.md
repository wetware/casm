# GSurv (Gradual Surveyer)

Gradual Surveyer is a service for discovering peers from a namespace by surveying incrementally longer distance nodes

| Lifecycle Stage | Maturity       | Status | Latest Revision |
|-----------------|----------------|--------|-----------------|
| 1A              | Working Draft  | Active | r1, 2022-01-27  |

Authors: [@aratz-lasa], [@lthibault]

[@aratz-lasa]: https://github.com/aratz-lasa
[@lthibault]: https://github.com/lthibault

See libp2p's [lifecycle document][lifecycle-spec] for context about maturity level
and spec status.

[lifecycle-spec]: https://github.com/libp2p/specs/blob/master/00-framework-01-spec-lifecycle.md

## Table of Contents

- [GSurv (Gradual Surveyer)](#gsurv-gradual-surveyer)
  - [Table of Contents](#table-of-contents)
  - [Motivation](#motivation)
  - [Protocol Specification](#protocol-specification)

## Motivation
When a node wants to connect to a namespace, it needs to find peers that are already part of the namespace. For finding other peers it usually makes use of a central bootstrap server. However, a bootstrap server introduces extra complexity to the system. Moreover, there are cases where the node is deployed in a cluster, where other peers are found in the same local network e.g. AWS cluster. That is why, a lightweight and simple protocol for discovering peers by querying other peers is desired.

Currently, Libp2p offers a protocol that provides service discovery in the local network through mDNS (Multicast DNS). However, this protocol may lead to scalability issues as indicated in [RFC 7558](https://datatracker.ietf.org/doc/html/rfc7558). One of the main scabality issues arises when there are many peers in the local network that receive the multicast request. Each of the receiver nodes respond to the multicast, overloading the request initiator. Therefore, an alternative protocol is presented here, in order to reduce the burstiness of mDNS.

The alternative protocol should avoid all the receivers to respond at once. Instead, an incremental (gradual) approach is desired, where based on a logical distance to the requester, the receivers decide to respond or not. In every message, the requester specifies the maximum distance at which a node that responds should be. If not enough responses are received, then, the distance is increased and a new request is multicasted. The distance is increased until enough responses are collected.

## Protocol Specification
GSurv is a protocol for discovering peers from a namespace. It makes use of multicasting for sending discovery surveys to other peers.

### Conventions

>The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT", "SHOULD", "SHOULD NOT", "RECOMMENDED",  "MAY", and "OPTIONAL" in this document are to be interpreted as described in [RFC 2119](https://datatracker.ietf.org/doc/html/rfc2119) and [RFC 2119](https://datatracker.ietf.org/doc/html/rfc8174).

### Definitions
- **Requester**: the node that wants to discover peers from a namespace. That is to say, the node that sends a multicast message as a request for disovering peers.
- **Responder**: the node that receives a discovery request and is able to respond with known peers in the specified namespace.
- **Survey**: a communication pattern where a node A sends a multicast message, and it receives multiple responses. So, instead of request-response, it is request-multiresponse.
- **Namespace**: a logical grouping of nodes based on a string identifier called *namespace*.

### Protocol description
GSurv is a simpe request-multiresponse protocol, that gradually surveys farther peers. The protocol is divided into three steps:

1. Node A wants to find peers in namespace N. Then, it multicasts a message as a request for information, to the nodes in the local network. The expected information is simply a list of peers that are known to be in namespace N.
2. Node B receives A's request, and checks whether it is within the namespace N. If it is, it multicasts a message containing its peer information. If it is not part of namespace N, it simply ignores the message.
3. Node A receives B's response and joins the namespace N through B.

These steps are a high-level representation of the protocol and they ignore the _logic distance_, which is the key concept for making GSurv scalable.

In steps 1 and 2, the concept of _distance_ is used for incrementally discovering peers in namesapce N. In step 1, Node A includes a field that represents a _logic distance_, which is the maximum logical distance that responder nodes must be. Therefore, in step 2, when Node B receives the request, it first checks whether it is within the logical distance. Node B only responds to the request if it is within the distance. This way, if Node A does not receive (enough) responses, it increases the logical distance and multicasts again the request. Node A repeats step 1 increasing the logical distance until receives (enough) responses.

### Logical distance
As explained in [Protocol description](#protocol-description), the logical distance is used for incrementally surveying more nodes. But how is the logical distance calculated?

The logical distance is between node A and B is calculated by XORing the last 4 bytes of their `peer.ID`s. The maximum logical distance is specified as a `uint8`. After XORing `peer.ID`s, the result is logically right shiftted by the maximum logical distance. If the final result is 0, node B is within the maximum logical distance of A, otherwise it is outside.

**Example - within maximum distance:**
```
Node A ID (last 4 bytes) = 1110000110100000
Node B ID (last 4 bytes) = 1110000111100000
Maximum distance = 8

1110 0001 1010 0000 XOR 1110 0001 1110 0000 = 0000 0000 0100 0000
0000 0000 0100 0000 >> 8 = 0000 0000 0000 0000 =base10= 0
```

**Example - outside maximum distance:**
```
Node A ID (last 4 bytes) = 1110000110100000
Node B ID (last 4 bytes) = 1110000111100000
Maximum distance = 5

1110 0001 1010 0000 XOR 1110 0001 1110 0000 = 0000 0000 0100 0000
0000 0000 0100 0000 >> 5 = 0000 0000 0000 0010 =base10= 2 
```

### API
GSurv provides a `discovery.Discovery` Libp2p interface.

```go
type Discovery interface {
	FindPeers(ctx context.Context, ns string, opts ...Option) (<-chan peer.AddrInfo, error)
  Advertise(ctx context.Context, ns string, opts ...Option) (time.Duration, error)
}
```

### Wire format
GSurv encodes request and response messages using Capnproto. Capnproto si similar to Protocol Buffers but faster. Check [https://capnproto.org/](https://capnproto.org/) for understanding Capnproto encoding.

The GSurv request Capnproto format is the following:

```
+--------------+------------------+------------------------+
| src (bytes)  | distance (uint8) | namespace (text UTF-8) |
+--------------+------------------+------------------------+
```

_src_ is a signed `peer.PeerRecord` that specifies who was the requester. The PeerRecord MUST be signed, this way the receiver can validate the identity of the requester. _distance_ specifies the maximum logical distance at which the responder MUST be. Lastly, the _namespace_ is the namespace from which peers wants to be found.

The GSurv response Capnproto format is the following:

```
+-------------------+-------------------------+
| namespace (UTF-8) | envelope (bytes) |
+-------------------+-------------------------+
```

The _namespace_ is the namespace from which peers wants to be found. _envelope_ is a signed `peer.PeerRecord`s that is from the specified namespace.

## Known Issues

None (...so far!)

## Alternative designs
TODO:

## Core Team

- [@lthibault](https://github.com/lthibault)
- [@aratz-lasa](https://github.com/aratz-lasa) ★

★ Project Lead

## References
1. S. Cheshire and M. Krochmal. Multicast DNS. RFC 6762, Feb. 2013.
