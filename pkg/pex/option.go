package pex

import (
	"time"

	"github.com/lthibault/log"
)

type Option func(pex *PeerExchange)

// WithLogger sets the logger for the peer exchange.
// If l == nil, a default logger is used.
func WithLogger(l log.Logger) Option {
	if l == nil {
		l = log.New()
	}

	return func(pex *PeerExchange) {
		pex.log = logger{l.WithField("id", pex.h.ID())}
	}
}

// WithTick sets the interval between gossip rounds.
// A lower value of 'd' improves cluster resiliency
// at the cost of increased bandwidth usage.
//
// If d <= 0, a default value of 1m is used.  Users
// SHOULD NOT alter this value without good reason.
func WithTick(d time.Duration) Option {
	if d <= 0 {
		d = time.Minute
	}

	return func(pex *PeerExchange) {
		pex.tick = d
	}
}

func withDefaults(opt []Option) []Option {
	return append([]Option{
		WithLogger(nil),
		WithTick(-1),
	}, opt...)
}
