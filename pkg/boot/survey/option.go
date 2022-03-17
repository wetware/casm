package survey

import (
	"github.com/lthibault/log"

	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/wetware/casm/pkg/boot/util"
)

type Option func(*Surveyor)

func WithLogger(l log.Logger) Option {
	if l == nil {
		l = log.New()
	}

	return func(s *Surveyor) {
		s.log = l
	}
}

func WithTransport(t util.Transport) Option {
	return func(s *Surveyor) {
		s.tp = t
	}
}

func withDefaults(opt []Option) []Option {
	return append([]Option{
		WithLogger(nil),
		WithTransport(util.UdpTransport),
	}, opt...)
}

/*
	discovery.Discovery options ...
*/

type (
	keyDistance struct{}
)

func distance(o discovery.Options) uint8 {
	if d, ok := o.Other[keyDistance{}].(uint8); ok {
		return d
	}

	return 255
}

// option for specifying distance when calling FindPeers
func WithDistance(dist uint8) discovery.Option {
	return func(opts *discovery.Options) error {
		opts.Other = make(map[interface{}]interface{})
		opts.Other[keyDistance{}] = dist
		return nil
	}
}
