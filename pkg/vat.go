package casm

import (
	"context"
	"errors"
	"fmt"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"

	"capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/exc"
	"capnproto.org/go/capnp/v3/rpc"
	"github.com/lthibault/log"
)

var ErrInvalidNS = errors.New("invalid namespace")

// Vat wraps a libp2p Host and provides a high-level interface to a
// capability-oriented network. Host has no private fields, and can
// be instantiated directly. The New() function is also provided as
// convenient way of populating the Host field.
type Vat struct {
	NS      string
	Host    host.Host
	Metrics MetricReporter
	Logger  log.Logger
}

// New is a convenience method that constructs a libp2p host and uses
// it to populate the Vat's Host field.   Callers MUST NOT mutate any
// of the returned Vat's fields after calling its methods.
func New(ns string, f HostFactory) (Vat, error) {
	if ns == "" {
		ns = "casm"
	}

	h, err := f()
	return Vat{
		NS:   ns,
		Host: h,
	}, err
}

func (v Vat) Loggable() map[string]interface{} {
	return map[string]interface{}{
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
		if v.isInvalidNS(vat.ID, c) {
			return nil, ErrInvalidNS
		}

		return nil, err
	}

	return rpc.NewConn(c.Upgrade(s), &rpc.Options{
		BootstrapClient: bootstrapper(c),
		ErrorReporter:   v.errorReporter(s),
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

func (v Vat) errorReporter(s network.Stream) rpc.ErrorReporter {
	if v.Logger == nil {
		return nil
	}

	return streamErrorReporter{
		l: v.Logger.WithField("stream", s.ID()),
	}
}

func (v Vat) metrics() metricsReporter {
	return metricsReporter{v.Metrics}
}

// match the protocol, ignoring namespace
func matches(c Capability, proto protocol.ID) bool {
	for _, p := range c.Protocols() {
		if hasSuffix(proto, p) {
			return true
		}
	}

	return false
}

func hasSuffix(s, suffix protocol.ID) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
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
		m.Incr(fmt.Sprintf("rpc.%s", id))
		m.Incr("rpc.connected")
	}

}

func (m metricsReporter) StreamClosed(id protocol.ID) {
	if m.MetricReporter != nil {
		m.Decr(fmt.Sprintf("rpc.%s", id))
		m.Decr("rpc.connected")
	}
}

type streamErrorReporter struct {
	l log.Logger
}

func (r streamErrorReporter) ReportError(err error) {
	if r.l != nil && err != nil {
		log := r.l.WithError(err)

		switch ex := err.(type) {
		case exc.Exception:
			log = log.WithField("exc_type", ex.Type)
		case *exc.Exception:
			if ex == nil { // BUG: capnp sometimes passes nil *Exception
				return
			}

			log = log.WithField("exc_type", ex.Type)
		}

		log.Debug("error encountered in rpc protocol")
	}
}
