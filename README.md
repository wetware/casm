# 🧬 CASM
**Cluster Assembly.**  Modular middleware for distributed computing

[![GoDoc](https://godoc.org/github.com/wetware/casm?status.svg)](https://godoc.org/github.com/wetware/casm)
[![Go Report Card](https://goreportcard.com/badge/github.com/wetware/casm?style=flat-square)](https://goreportcard.com/report/github.com/wetware/casm)
[![Go](https://github.com/wetware/casm/actions/workflows/go.yml/badge.svg)](https://github.com/wetware/casm/actions/workflows/go.yml)
[![Matrix](https://img.shields.io/matrix/wetware:matrix.org?color=lightpink&label=Live%20Support&logo=matrix&style=flat-square)](https://matrix.to/#/#wetware:matrix.org)

## What is CASM?

CASM is short for **Cluster Assembly**.  It is a low-level toolkit for developing efficient, reliable and secure distributed systems.  It is entirely peer-to-peer and requires no coordinator nodes or other infrastructure.  It is built using [libp2p](https://libp2p.io/) and integrates seamlessly into the Protocol Labs ecosystem.

### Design

CASM appeals to developers in search of firm ground on which to build distributed systems.  It offers zero-cost abstractions<sup>1</sup> that put you in control of trade-offs, while enforcing key properties of well-behaved systems.

In particular, the following invariants are preserved throughout the public API:

1.  Cluster membership is dynamic.
2.  Data is automatically signed and validated.
3.  Network protocols are [partition-available and low-latency](https://en.wikipedia.org/wiki/PACELC_theorem).
4.  Security is provided through [Object Capabilities](https://en.wikipedia.org/wiki/Capability-based_security).

Users can stack additional guarantees in application logic.  For example, you can build a consistent database out of CASM parts.

### Features

CASM follows a modular, "Lego bricks" design, allowing you to pick and choose the pieces you want.

| Feature        | Package       | Description |
| -------------- | ------------- | ----------- |
| RPC            | `pkg/`        | Fast & extensible RPC for communicating between nodes, with capability-based security. |
| Bootstrap      | `pkg/boot`    | Pluggable strategies for discovering and joining clusters. |
| Peer Exchange  | `pkg/pex`     | Lightweight gossip-based protocol for randomly sampling peers.  Ideal for building caches. |
| Clustering     | `pkg/cluster` | Unstructured service providing a global view of the cluster<sup>2</sup>. |
| Live Debugging | `pkg/debug`   | Production-ready integrations for `runtime/debug`, `runtime/pprof` and `runtime/trace`. |

## Getting Started

### Installation

Run `go get github.com/wetware/casm` with modules enabled.

### Getting Support

The best place to get help is on [Matrix](https://matrix.to/#/!qsAqxgSQYuowuCsigM:matrix.org?via=matrix.org), or by asking a question on the [Wetware Q&A Board](https://github.com/wetware/ww/discussions/categories/q-a).

We're friendly! Drop in and say hi! 👋

## CASM Users

Organizations using CASM in production

<!-- prettier-ignore-start -->
<!-- markdownlint-disable -->
<table>
  <tbody>
    <tr>
      <td align="center"><a href="https://blocknative.com"><img src="https://avatars.githubusercontent.com/u/40773874?s=100" width="100px;" alt="Blocknative"/><br /><sub><b>Blocknative</b></sub></a><br /></td>
    </tr>
  </tbody>
</table>

<!-- markdownlint-restore -->
<!-- prettier-ignore-end -->

## Contributors

<a href="https://github.com/wetware/casm/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=wetware/casm" />
</a>

## Footnotes

1. The term "zero-cost" is obviously a figure of speech, which is intended to emphasize the following point.  As a matter of principle, CASM emphasizes "thin", non-leaky abstractions that do not significantly impact performance.
2. In the spirit of zero-cost abstractions, CASM's clustering protocol provides [PA/EL](https://en.wikipedia.org/wiki/PACELC_theorem) guarantees.  No effort is made to provide a consistent view between nodes, because (a) it is rarely needed in practice, and (b) this configuration provides you with the greatest flexibility.  CASM provides an ideal foundation on which to build more specialized (including consistent) systems.

## References

- UnsServ:  Unstructured Peer-to-Peer Services [[pdf](https://aratz.lasa.eus/file/unsserv.pdf)]
