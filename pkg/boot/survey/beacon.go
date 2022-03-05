package survey

import (
	"context"
	"time"

	"github.com/libp2p/go-libp2p-core/discovery"
	ps "github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/lthibault/log"
)

type Beacon struct {
	Logger log.Logger
}

func (b Beacon) Advertise(ctx context.Context, ns string, opt ...discovery.Option) (time.Duration, error) {
	b.Logger.WithField("ttl", ps.PermanentAddrTTL).
		Warn("stub call to advertise returned")

	return ps.PermanentAddrTTL, nil
}
