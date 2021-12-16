# CASM
Universal middleware for decentralized computing

[![GoDoc](https://godoc.org/github.com/wetware/casm?status.svg)](https://godoc.org/github.com/wetware/casm)
[![Go](https://github.com/wetware/casm/actions/workflows/go.yml/badge.svg)](https://github.com/wetware/casm/actions/workflows/go.yml)

## What is CASM?

CASM is short for **Cluster Assembly**.  It is a low-level toolkit for developing efficient, reliable and secure peer-to-peer systems.  It is built using [libp2p](https://libp2p.io/) and integrates seamlessly into the Protocol Labs ecosystem.

CASM appeals to developers in search of firm ground on which to build distributed systems.  It offers a set of zero-cost abstractions<sup>1</sup> that put developers in control of trade-offs, while enforcing the minimum set of guarantees needed for well-behaved systems.

In particular, the following properties can be safely assumed throughout the public API:

1.  Cluster membership is dynamic.
2.  The cluster is inconsistent (CASM is a [PA/EL](https://en.wikipedia.org/wiki/PACELC_theorem) system).
3.  All data transfer is authenticated (through not necessarily encrypted).

Users can stack additional guarantees in application logic, for example by building a consistency protocol out of CASM parts.

## Getting Started

### Installation

Run `go get github.com/wetware/casm@latest` with modules enabled.

### Usage

CASM's functionality is grouped by package.  Developers will typically import one items under `pkg/*`, depending on application needs.  The following functionality is provided:

- `pkg/boot`:  network crawling and other strategies for ambient peer discovery.
- `pkg/pex`:  efficient, resilient gossip-based peer sampling protocol.
- `pkg/cluster`:  unstructured clustering service with PA/EL guarantees.

Planned features include:

- [ ] CSP abstractions for scheduling and synchronizing processes

## Footnotes

1. The term "zero-cost" is obviously a figure of speech, which is intended to emphasize the following point.  As a matter of principle, CASM emphasizes "thin", non-leaky abstractions that do not significantly impact performance.

## References

- UnsServ:  Unstructured Peer-to-Peer Services [[pdf](https://aratz.lasa.eus/file/unsserv.pdf)]
