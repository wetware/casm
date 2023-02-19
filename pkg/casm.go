//go:generate mockgen -source=casm.go -destination=../internal/mock/pkg/casm.go -package=mock_casm

package casm

import (
	"sync"

	"capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/rpc"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/protocol"
	quic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"
	"github.com/pierrec/lz4/v4"
	protoutil "github.com/wetware/casm/pkg/util/proto"
)

const (
	Version             = "0.0.0"
	Proto   protocol.ID = "/casm/" + Version
)

var match = protoutil.Match(
	protoutil.Prefix("casm"),
	protoutil.SemVer(Version))

// Subprotocol returns a protocol.ID that matches the
// pattern:  /casm/<version>/<ns>/<...>
func Subprotocol(ns string, ss ...string) protocol.ID {
	return protoutil.AppendStrings(Proto,
		append([]string{ns}, ss...)...)
}

// NewMatcher returns a stream matcher for a protocol.ID
// that matches the pattern:  /casm/<version>/<ns>
func NewMatcher(ns string) protoutil.MatchFunc {
	return match.Then(protoutil.Exactly(ns))
}

// Stream is a full-duplex byte-stream with reliable delivery semantics.
type Stream interface {
	Protocol() protocol.ID
	Read([]byte) (int, error)
	Write([]byte) (int, error)
	Close() error
}

type ClientProvider interface {
	// Client returns the client capability to be exported.  It is called
	// once for each incoming Stream, so implementations may either share
	// a single global object, or instantiate a new object for each call.
	Client() capnp.Client
}

// Bootstrapper is an optional interface provided by Capability types,
// which provides a bootstrap interface.  The capability returned by
// Bootstrap will be made available to the remote end of a Stream.
type Bootstrapper interface {
	Bootstrap() capnp.Client
}

// MetricReporter is used to track open RPC connections.
type MetricReporter interface {
	Incr(key string)
	Decr(key string)
}

// HostFactory constructs a libp2p host.
type HostFactory func() (host.Host, error)

// Client returns a HostFactory for a client host.   A client host
// has no listen address, and does not accept incoming connections.
// Options are passed through to the underlying call to libp2p.New.
func Client(opt ...libp2p.Option) HostFactory {
	return factory(opt, []libp2p.Option{
		libp2p.NoTransports,
		libp2p.NoListenAddrs,
		libp2p.Transport(tcp.NewTCPTransport),
		libp2p.Transport(quic.NewTransport)})
}

// Client returns a HostFactory for a server host.  A server host
// has listen addresses and accepts incoming connections from the
// network.  Options are passed through to the underlying call to
// libp2p.New.
func Server(opt ...libp2p.Option) HostFactory {
	return factory(opt, []libp2p.Option{
		libp2p.NoTransports,
		libp2p.Transport(tcp.NewTCPTransport),
		libp2p.Transport(quic.NewTransport)})
}

func factory(opt, defaults []libp2p.Option) HostFactory {
	return func() (host.Host, error) {
		return libp2p.New(append(defaults, opt...)...)
	}
}

type Capability interface {
	// Protocols returns the IDs for the given capability.
	// Implementations SHOULD order protocol identifiers in decreasing
	// order of priority.
	Protocols() []protocol.ID

	// Upgrade a raw byte-stream to an RPC transport.  Implementations
	// MAY select a Transport impmlementation based on the protocol ID
	// returned by 'Stream.Protocol'.
	Upgrade(Stream) rpc.Transport
}

// BasicCap is a basic provider of Capability. Most implementations
// will benefit from using this.  Protocol IDs SHOULD be ordered in
// descending order of preference, i.e.: lower-indexed protocol IDs
// will be used preferrentially.
//
// BasicCap automatically parses and recognizes two suffixes:
//
// - /packed :: causes Upgrade to use a packed Cap'n Proto encoding.
// - /lz4    :: causes Upgrade to use an LZ4 compressed stream.
//
// The '/lz4' suffix may follow '/packed', i.e.:  '/packed/lz4' is
// supported, whereas '/lz4/packed' is not.
type BasicCap []protocol.ID

// Lists all protocol IDs that match capability c, in order of
// precedence.
func (c BasicCap) Protocols() []protocol.ID { return c }

// Upgrade a libp2p Stream to a capnp Transport.
func (c BasicCap) Upgrade(s Stream) rpc.Transport {
	if MatchLz4(s.Protocol()) {
		return c.Upgrade(&Lz4Stream{Stream: s})
	}

	if MatchPacked(s.Protocol()) {
		return rpc.NewPackedStreamTransport(s)
	}

	return rpc.NewStreamTransport(s)
}

var (
	lz4Prot    = protoutil.Suffix("lz4")
	packedProt = protoutil.Suffix("packed")
)

// MatchPacked returns true if the supplied protocol.ID requires
// a packed Cap'n Proto transport.
func MatchPacked(id protocol.ID) bool {
	return packedProt.Match(id)
}

// MatchLz4 returns true if the supplied protocol.ID requires
// a Lz4 compressed Cap'n Proto transport.
func MatchLz4(id protocol.ID) bool {
	return lz4Prot.Match(id)
}

type Lz4Stream struct {
	Stream

	once sync.Once
	r    *lz4.Reader
	w    *lz4.Writer
}

func (s *Lz4Stream) Protocol() protocol.ID {
	base, _ := protoutil.Split(s.Stream.Protocol())
	return base
}

func (s *Lz4Stream) Read(b []byte) (int, error) {
	s.init()
	return s.r.Read(b)
}

func (s *Lz4Stream) Write(b []byte) (int, error) {
	s.init()
	return s.w.Write(b)
}

func (s *Lz4Stream) Close() error {
	return s.Stream.Close()
}

func (s *Lz4Stream) init() {
	s.once.Do(func() {
		s.r = lz4.NewReader(s.Stream)
		s.w = lz4.NewWriter(s.Stream)
	})
}
