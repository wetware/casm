package survey

import (
	"github.com/lthibault/log"
	"github.com/wetware/casm/pkg/boot/socket"

	"github.com/libp2p/go-libp2p-core/discovery"
)

type Option func(*Surveyor)

// WithLogger sets the logger instance.
// If l == nil, a default logger is used.
func WithLogger(l log.Logger) Option {
	if l == nil {
		l = log.New()
	}

	return func(s *Surveyor) {
		s.log = l
	}
}

// WithCacheSize sets the number the size of the response-
// record cache.  Set this to the number of namespaces the
// host is expected to join.
//
// If size <= 0, cache size defaults to 8.
func WithCacheSize(size int) Option {
	if size <= 0 {
		size = 8
	}

	return func(s *Surveyor) {
		s.cache = socket.NewRecordCache(size)
	}
}

func withDefaults(opt []Option) []Option {
	return append([]Option{
		WithLogger(nil),
		WithCacheSize(-1),
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
