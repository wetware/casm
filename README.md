# CASM
Universal middleware for decentralized computing

[![GoDoc](https://godoc.org/github.com/wetware/casm?status.svg)](https://godoc.org/github.com/wetware/casm)
[![Go](https://github.com/wetware/casm/actions/workflows/go.yml/badge.svg)](https://github.com/wetware/casm/actions/workflows/go.yml)

**Cluster Assembly** (CASM) is a library for writing peer-to-peer applications using [Communicating Sequential Processes](https://en.wikipedia.org/wiki/Communicating_sequential_processes) (CSP).  It provides "process" and "channel" objects that interact across hosts over the internet, allowing users to build decentralized applications that are also intuitive to manage at scale.

CASM is built on top [libp2p](https://libp2p.io/) and integrates seamlessly into the larger Protocol Labs ecosystem.
