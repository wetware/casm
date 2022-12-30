package casm

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"
	"unsafe"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/multiformats/go-multistream"

	"capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/rpc"
)

var ErrInvalidNS = errors.New("invalid namespace")

// ID is an opaque identifier that identifies a unique vat on the
// on the network.  A fresh identifier SHOULD be created for each
// host restart, such that individual runs of the libp2p host may
// be distinguished.  Use of a cryptographic PRNG is NOT REQUIRED.
type ID uint64

// String returns the hexadecimal representation of the ID.
func (id ID) String() string {
	b, _ := id.MarshalText()
	return *(*string)(unsafe.Pointer(&b))
}

// Bytes returns the ID as a byte array.
func (id ID) Bytes() []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(id))
	return buf
}

func (id *ID) UnmarshalText(b []byte) error {
	if len(b) != 16 {
		return fmt.Errorf("invalid length: expected 16, got %d", len(b))
	}

	var buf [8]byte
	_, err := hex.Decode(buf[:], b)
	if err == nil {
		*(*uint64)(id) = binary.BigEndian.Uint64(buf[:])
	}

	return err
}

func (id ID) MarshalText() ([]byte, error) {
	buf := make([]byte, 16)
	binary.BigEndian.PutUint64(buf[8:], uint64(id))
	hex.Encode(buf, buf[8:])
	return buf, nil
}

func (id ID) Loggable() map[string]any {
	return map[string]any{
		"server": id,
	}
}

// Vat wraps a libp2p Host and provides a high-level interface to a
// capability-oriented network. Host has no private fields, and can
// be instantiated directly. The New() function is also provided as
// convenient way of populating the Host field.
type Vat struct {
	ID      ID
	NS      string
	Host    host.Host
	Metrics MetricReporter
}

// New is a convenience method that constructs a libp2p host and uses
// it to populate the Vat's Host field.  The Metrics field MAY be set
// manually before any of the returned Vat's methods are called.  The
// ID field is generated using math/rand.
func New(ns string, f HostFactory) (Vat, error) {
	if ns == "" {
		ns = "casm"
	}

	// Use throw-away source to avoid seeding the global source in
	// math/rand.
	source := rand.NewSource(time.Now().UnixNano()).(rand.Source64)

	h, err := f()
	return Vat{
		ID:   ID(source.Uint64()),
		NS:   ns,
		Host: h,
	}, err
}

func (v Vat) Loggable() map[string]interface{} {
	return map[string]interface{}{
		"id":   v.ID,
		"ns":   v.NS,
		"peer": v.Host.ID(),
	}
}

// Connect to a capability hostend on vat.  The context is used only
// when negotiating network connections and is safe to cancel when a
// call to 'Connect' returns. The RPC connection is returned without
// waiting for the remote capability to resolve.  Users MAY refer to
// the 'Bootstrap' method on 'rpc.Conn' to resolve the connection.
//
// The 'Addrs' field of 'vat' MAY be empty, in which case the network
// will will attempt to discover a valid address.
//
// If 'c' satisfies the 'Bootstrapper' interface, the client returned
// by 'c.Bootstrap()' is provided to the RPC connection as a bootstrap
// capability.
func (v Vat) Connect(ctx context.Context, vat peer.AddrInfo, c Capability) (*rpc.Conn, error) {
	if len(vat.Addrs) > 0 {
		if err := v.Host.Connect(ctx, vat); err != nil {
			return nil, err
		}
	}

	s, err := v.Host.NewStream(ctx, vat.ID, v.protocolsFor(c)...)
	if err != nil {
		if err != multistream.ErrNotSupported {
			return nil, err
		}

		if v.isInvalidNS(vat.ID, c) {
			return nil, ErrInvalidNS
		}

		return nil, err // TODO:  catch multistream.ErrNotSupported
	}

	return rpc.NewConn(c.Upgrade(s), &rpc.Options{
		BootstrapClient: bootstrapper(c),
	}), nil
}

// Export a capability, making it available to other vats in the network.
func (v Vat) Export(c Capability, boot ClientProvider) {
	for _, id := range v.protocolsFor(c) {
		v.handle(c, boot, id)
	}
}

// Embargo ceases to export 'c'.  New calls to 'Connect' are guaranteed
// to fail for 'c' after 'Embargo' returns. Existing RPC connections on
// 'c' are unaffected.
//
// CAUTION: Embargo is asynchronous.  The capability MAY NOT be disabled
// when Embargo() returns.  This will be fixed in the future.
func (v Vat) Embargo(c Capability) {
	for _, id := range v.protocolsFor(c) {
		// TODO(security):  RemoveStreamHandler is asynchronous.  Can we
		//                  wait for an event on the event.Bus before we
		//					return?
		v.Host.RemoveStreamHandler(id)
	}
}

func (v Vat) protocolsFor(c Capability) (ps []protocol.ID) {
	for _, id := range protocol.ConvertToStrings(c.Protocols()) {
		ps = append(ps, Subprotocol(v.NS, id))
	}

	return
}

func (v Vat) handle(c Capability, boot ClientProvider, id protocol.ID) {
	v.Host.SetStreamHandler(id, func(s network.Stream) {
		defer s.Close()

		v.metrics().StreamOpened(id)
		defer v.metrics().StreamClosed(id)

		conn := rpc.NewConn(c.Upgrade(s), &rpc.Options{
			BootstrapClient: boot.Client(),
		})
		defer conn.Close()

		<-conn.Done()
	})
}

func (v Vat) isInvalidNS(id peer.ID, c Capability) bool {
	ps, err := v.Host.Peerstore().GetProtocols(id)
	if err == nil {
		for _, proto := range ps {
			if matches(c, proto) {
				// the remote peer supports the capability, so it
				// has to be a namespace mismatch.
				return true
			}
		}
	}

	return false // not a ns issue; proto actually unsupported
}

func (v Vat) metrics() metricsReporter {
	return metricsReporter{v.Metrics}
}

// match the protocol, ignoring namespace
func matches(c Capability, proto string) bool {
	for _, p := range c.Protocols() {
		if strings.HasSuffix(proto, string(p)) {
			return true
		}
	}

	return false
}

func bootstrapper(c Capability) capnp.Client {
	if b, ok := c.(Bootstrapper); ok {
		return b.Bootstrap()
	}

	return capnp.Client{}
}

type metricsReporter struct{ MetricReporter }

func (m metricsReporter) StreamOpened(id protocol.ID) {
	if m.MetricReporter != nil {
		m.Incr(fmt.Sprintf("rpc.%s.open", id))
		m.Incr("rpc.connect")
	}

}

func (m metricsReporter) StreamClosed(id protocol.ID) {
	if m.MetricReporter != nil {
		m.Decr(fmt.Sprintf("rpc.%s.open", id))
		m.Incr("rpc.disconnect")
	}
}
