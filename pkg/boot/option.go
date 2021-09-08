package boot

import (
	"github.com/lthibault/log"
	ma "github.com/multiformats/go-multiaddr"
	"go.uber.org/fx"
)

// Config provides options to the dependency-injection
// framework.
type Config struct {
	fx.Out

	Log          log.Logger
	Chan         chan outgoing
	NewTransport transportFactory
}

func (c *Config) Apply(opt []Option) {
	for _, option := range withDefaults(opt) {
		option(c)
	}
}

// Option type for bootstrap services.
type Option func(c *Config)

// WithLogger sets the logger.
func WithLogger(l log.Logger) Option {
	if l == nil {
		l = log.New()
	}

	return func(c *Config) {
		c.Log = l
	}
}

// WithTransport sets the transport.  If t == nil,
// defaults to UDP multicast using grop 228.8.8.8:8822.
func WithTransport(t Transport) Option {
	if t == nil {
		return withTransportFactory(nil)
	}

	return withTransportFactory(func() (Transport, error) {
		return t, nil
	})
}

func withTransportFactory(f transportFactory) Option {
	const m = "/ip4/228.8.8.8/udp/8822"

	if f == nil {
		f = func() (Transport, error) {
			return NewMulticastUDP(ma.StringCast(m))
		}
	}

	return func(c *Config) {
		c.NewTransport = f
	}
}

func withSendBuffer(n int) Option {
	return func(c *Config) {
		c.Chan = make(chan outgoing, n)
	}
}

func withDefaults(opt []Option) []Option {
	return append([]Option{
		WithLogger(nil),
		WithTransport(nil),
		withSendBuffer(8),
	}, opt...)
}
