package survey

import (
	"github.com/lthibault/log"

	"github.com/libp2p/go-libp2p-core/discovery"
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

func WithTransport(t Transport) Option {
	return func(s *Surveyor) {
		s.t = t
	}
}

func withDefaults(opt []Option) []Option {
	return append([]Option{
		WithLogger(nil),
		WithTransport(Transport{}),
	}, opt...)
}

/*
	discovery.Discovery options ...
*/

// option for specifying distance when calling FindPeers
func WithDistance(dist uint8) discovery.Option {
	return func(opts *discovery.Options) error {
		opts.Other = make(map[interface{}]interface{})
		opts.Other["distance"] = dist
		return nil
	}
}
