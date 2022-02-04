package survey

import (
	"context"
	"time"

	"github.com/jpillora/backoff"
	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/wetware/casm/internal/api/survey"
)

// GradualSurveyor queries a progressively larger subset of the
// cluster keyspace.  It is recommended for bootstrapping large
// clusters where multicast storms may degrade the service.
//
// See specs/survey.md for details.
type GradualSurveyor struct {
	// Factor controls how long the GradualSurveyor will wait for
	// peers at each distance. The raw wait time is calculated by
	// evaluating 'distance * factor', before applying min, max &
	// jitter.  Defaults to 1.5.
	Factor float64

	// Min and Max wait durations.  Defaults to 5s and 90s.
	Min, Max time.Duration

	// If true, use the absolute wait durations calculated via min,
	// max and factor.  By default, the final wait time is sampled
	// uniformly from the interval (w/2, w), where 'w' is the wait
	// duration.
	DisableJitter bool

	*Surveyor
}

func (g GradualSurveyor) FindPeers(ctx context.Context, ns string, opt ...discovery.Option) (<-chan peer.AddrInfo, error) {
	p, err := g.buildRequest(ns, 0)
	if err != nil {
		return nil, err
	}

	found, err := g.Surveyor.FindPeers(ctx, ns, append(opt, WithDistance(0))...)
	if err != nil {
		return nil, err
	}

	out := make(chan peer.AddrInfo, 1)
	go func() {
		defer close(out)

		b := backoff.Backoff{
			Factor: g.factor(),
			Min:    g.min(),
			Max:    g.max(),
			Jitter: !g.DisableJitter,
		}

		for {
			select {
			case <-time.After(b.Duration()):
				err := g.retry(ctx, p, uint8(b.Attempt()))
				if err != nil {
					g.log.WithError(err).Debug("retry failed")
				}
				continue

			case info := <-found:
				out <- info

			case <-ctx.Done():
				return
			}

			break
		}

		for info := range found {
			select {
			case out <- info:
			case <-ctx.Done():
			}
		}
	}()

	return out, nil
}

func (g GradualSurveyor) factor() float64 {
	if g.Factor == 0 {
		return 1.5
	}
	return g.Factor
}

func (g GradualSurveyor) min() time.Duration {
	if g.Min == 0 {
		return time.Second * 5
	}
	return g.Min
}

func (g GradualSurveyor) max() time.Duration {
	if g.Max == 0 {
		return time.Second * 90
	}
	return g.Max
}

func (g GradualSurveyor) retry(ctx context.Context, p survey.Packet, d uint8) error {
	r, err := p.Request()
	if err == nil {
		r.SetDistance(d)
		err = g.emitRequest(ctx, p)
	}
	return err
}
