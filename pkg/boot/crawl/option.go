package crawl

import (
	"github.com/lthibault/log"
	"github.com/wetware/casm/pkg/boot/socket"
)

type Option func(*Crawler)

// WithLogger sets the logger instance.
// If l == nil, a default logger is used.
func WithLogger(l log.Logger) Option {
	if l == nil {
		l = log.New()
	}

	return func(c *Crawler) {
		c.log = l
	}
}

// WithStrategy specifies how crawl addresses are generated.
// If s == nil, defaults to scanning all local ports in the
// interval (1024, 65535).
func WithStrategy(s Strategy) Option {
	if s == nil {
		s = NewPortScan(nil, 0)
	}

	return func(c *Crawler) {
		c.iter = s
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

	return func(c *Crawler) {
		c.cache = socket.NewRecordCache(size)
	}
}

func withDefaults(opt []Option) []Option {
	return append([]Option{
		WithLogger(nil),
		WithStrategy(nil),
		WithCacheSize(-1),
	}, opt...)
}
