package cluster

import (
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/lthibault/log"
	"github.com/wetware/casm/pkg/cluster/pulse"
	"github.com/wetware/casm/pkg/cluster/routing"
)

type Option func(*Node)

func WithNamespace(ns string) Option {
	if ns == "" {
		ns = "casm"
	}

	return func(m *Node) {
		m.ns = ns
	}
}

func WithTTL(d time.Duration) Option {
	if d <= 0 {
		d = time.Second * 10
	}

	return func(m *Node) {
		m.a.ttl = d
	}
}

func WithLogger(l log.Logger) Option {
	if l == nil {
		l = log.New()
	}

	return func(m *Node) {
		m.a.log = l
	}
}

func WithRoutingTable(t RoutingTable) Option {
	if t == nil {
		t = routing.New()
	}

	return func(m *Node) {
		m.rt = t
	}
}

func WithMeta(meta pulse.Preparer) Option {
	if meta == nil {
		meta = defaultMeta{}
	}

	return func(m *Node) {
		m.a.p = meta
	}
}

// WithReadiness specifies a criterion for considering the model
// to be ready.  If r == nil, no criterion is applied and the is
// always considered ready.
func WithReadiness(r pubsub.RouterReady) Option {
	if r == nil {
		// r = pubsub.MinTopicSize(1)  // TODO:  consider enabling this
		r = func(pubsub.PubSubRouter, string) (bool, error) {
			return true, nil // nop
		}
	}

	return func(m *Node) {
		m.a.ready = r
	}
}

func withDefault(opt []Option) []Option {
	return append([]Option{
		WithNamespace(""),
		WithTTL(-1),
		WithLogger(nil),
		WithRoutingTable(nil),
		WithMeta(nil),
		WithReadiness(nil),
	}, opt...)
}

type defaultMeta struct{}

func (defaultMeta) Prepare(pulse.Heartbeat) {}
