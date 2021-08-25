package pex

import (
	"sync"
	"sync/atomic"

	"github.com/libp2p/go-eventbus"
	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"go.uber.org/multierr"

	"github.com/lthibault/log"
	"github.com/sirupsen/logrus"
)

/*
 * utils.go contains unexported utility types
 */

func lastbyte(id peer.ID) uint8 { return uint8(id[len(id)-1]) }

// breaker is a helper type that is used to elide repeated
// error checks.  Calls to Do() become a nop when Err != nil.
//
// See:  https://go.dev/blog/errors-are-values
type breaker struct{ Err error }

func (b *breaker) Do(f func()) {
	if b.Err == nil {
		f()
	}
}

type logger struct{ log.Logger }

func (l logger) WithStream(s network.Stream) logger {
	return l.WithConn(s.Conn()).
		WithField("stream", s.ID()).
		WithField("proto", s.Protocol())
}

func (l logger) WithConn(c network.Conn) logger {
	return l.WithField("conn", c.ID()).
		WithField("peer", c.RemotePeer())
}

func (l logger) With(v log.Loggable) logger               { return logger{l.Logger.With(v)} }
func (l logger) WithError(err error) logger               { return logger{l.Logger.WithError(err)} }
func (l logger) WithFields(fs logrus.Fields) logger       { return logger{l.Logger.WithFields(fs)} }
func (l logger) WithField(s string, v interface{}) logger { return logger{l.Logger.WithField(s, v)} }

type atomicValues struct {
	record atomicGossipRecord

	sync.Mutex // serialize writers to 'view'
	view       atomicView
}

func newAtomicValues(bus event.Bus) (vs atomicValues, err error) {
	if vs.view, err = newAtomicView(bus); err == nil {
		vs.record, err = newAtomicGossipRecord(bus)
	}
	return
}

func (vs *atomicValues) CloseAll() error {
	return multierr.Combine(
		vs.view.Close(),
		vs.record.Close(),
	)
}

type atomicGossipRecord struct {
	val        atomic.Value
	evtUpdated event.Emitter
}

func newAtomicGossipRecord(bus event.Bus) (atomicGossipRecord, error) {
	e, err := bus.Emitter(new(EvtLocalRecordUpdated), eventbus.Stateful)
	return atomicGossipRecord{evtUpdated: e}, err
}

func (rec *atomicGossipRecord) Load() GossipRecord { return rec.val.Load().(GossipRecord) }

func (rec *atomicGossipRecord) Store(g GossipRecord) error {
	rec.val.Store(g)
	ev := EvtLocalRecordUpdated(g) // copy
	return rec.evtUpdated.Emit(ev)
}

func (rec *atomicGossipRecord) Close() error { return rec.evtUpdated.Close() }

type atomicView struct {
	val        atomic.Value
	evtUpdated event.Emitter
}

func newAtomicView(bus event.Bus) (v atomicView, err error) {
	v.evtUpdated, err = bus.Emitter(new(EvtViewUpdated))
	v.val.Store(View{})
	return
}

func (av *atomicView) Load() (v View) {
	v, _ = av.val.Load().(View)
	return
}

func (av *atomicView) Store(v View) error {
	av.val.Store(v)
	evt := make(EvtViewUpdated, len(v))
	copy(evt, v)
	return av.evtUpdated.Emit(evt)
}

func (av *atomicView) Close() error { return av.evtUpdated.Close() }

// streamError is a sentinel value used by PeerExchange.gossip.
type streamError struct {
	Peer peer.ID
	error
}

func (err streamError) Loggable() map[string]interface{} {
	return map[string]interface{}{
		"peer":  err.Peer,
		"error": err.error,
	}
}
