package crawl

type Option func(*Crawler)

func WithTransport(t Transport) Option {
	return func(c *Crawler) {
		c.transport = t
	}
}

func withDefaults(opt []Option) []Option {
	return append([]Option{
		WithTransport(Transport{}),
	}, opt...)
}
