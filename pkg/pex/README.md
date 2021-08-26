# Peer Exchange (PeX)

Gossip-based sampling service for robust connectivity.

## Overview

Once a host has successfully joined a cluster, it ceases to rely on the bootstrap service for peer-discovery, and instead relies on the cluster itself to discover new peers.  This process of "ambient peer discovery" is provided as an unstructured service called PeX (short for peer exchange).  In contrast to DHT and PubSub-based methods, PeX can be used to repair cluster partitions, or to rejoin a cluster after having been disconnected.

See [PEX.md](../../specs/pex.md) for more details.

## Quickstart

```go
package main

import (
    "fmt"
    "context"

    "github.com/libp2p/go-libp2p"
    "github.com/wetware/casm/pkg/pex"
)

const ns = "example_namespace"

var ctx = context.Background()

func main() {
    h, _ := libp2p.New(ctx)    // start libp2p host
    px, _ := pex.New(ctx, ns)  // create peer exchange

    // Join an existing cluster and start gossiping.  The 'info' parameter
    // is a 'peer.AddrInfo' instance that points to a bootstrap peer. This
    // is usually obtained through a discovery service (not shown).
    _ = px.Join(ctx, info)

    // Print the local view.  The records can be used to repair partitions
    // or to reconnect to a cluster after becoming isolated.
    for _, rec := range px.View() {
        fmt.Println(rec.PeerID)
    }
}
```
