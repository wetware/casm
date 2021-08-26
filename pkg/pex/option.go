package pex

import (
	"math"
	"time"

	"github.com/libp2p/go-libp2p-core/host"
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

// WithSelector sets the view selector to be used
// during gossip rounds.  If sel == nil, a default
// hybrid strategy is used.
//
// Note that view selection strategy can have a
// drastic effect on performance and stability.
// Users SHOULD NOT use this option unless they
// know what they are doing.
func WithSelector(f ViewSelectorFactory) Option {
	if f == nil {
		const thresh = math.MaxUint64 / 2

		f = func(h host.Host, d DistanceProvider, maxSize int) ViewSelector {
			if d.Distance(h)/math.MaxUint64 > thresh {
				return RandSelector(nil).Then(TailSelector(maxSize))
			}

			return SortSelector().Then(TailSelector(maxSize))
		}
	}

	return func(pex *PeerExchange) {
		pex.newSelector = f
	}
}

// WithMaxViewSize sets the maximum size of the view.
//
// If n == 0, a default value of 32 is used.
//
// Users SHOULD ensure all nodes in a given cluster have
// the same maximum view size.
func WithMaxViewSize(n uint) Option {
	if n == 0 {
		n = 32
	}

	return func(pex *PeerExchange) {
		pex.maxSize = int(n)
	}
}

func withDefaults(opt []Option) []Option {
	return append([]Option{
		WithLogger(nil),
		WithTick(-1),
		WithSelector(nil),
		WithMaxViewSize(0),
	}, opt...)
}
