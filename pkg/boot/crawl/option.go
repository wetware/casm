package crawl

import (
	"github.com/wetware/casm/pkg/boot/util"
)

type Option func(*Crawler)

func WithTransport(t util.Transport) Option {
	return func(c *Crawler) {
		c.transport = t
	}
}

func withDefaults(opt []Option) []Option {
	return append([]Option{
		WithTransport(util.UdpTransport),
	}, opt...)
}
