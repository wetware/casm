package mudp

import (
	"net"

	"github.com/libp2p/go-libp2p-core/discovery"
)

// Config supplies options to the dependency-injection framework.
type Config struct {
	Addr   *net.UDPAddr
	Dial   DialFunc
	Listen ListenFunc
}

func (c *Config) Apply(opt []Option) {
	for _, option := range withDefaults(opt) {
		option(c)
	}
}

type Option func(c *Config)

func WithAddr(addr *net.UDPAddr) Option {
	if addr == nil {
		addr, _ = net.ResolveUDPAddr("udp4", multicastAddr)
	}

	return func(c *Config) {
		c.Addr = addr
	}
}

func WithDial(dial DialFunc) Option {
	if dial == nil {
		dial = func(laddr, raddr *net.UDPAddr) (*net.UDPConn, error) {
			return net.DialUDP("udp4", laddr, raddr)
		}
	}

	return func(c *Config) {
		c.Dial = dial
	}
}

func WithListen(listen ListenFunc) Option {
	if listen == nil {
		listen = func(laddr *net.UDPAddr) (*net.UDPConn, error) {
			return net.ListenMulticastUDP("udp4", nil, laddr)
		}
	}

	return func(c *Config) {
		c.Listen = listen
	}
}

func withDefaults(opt []Option) []Option {
	return append([]Option{
		WithAddr(nil),
		WithDial(nil),
		WithListen(nil),
	}, opt...)
}

// option for specifying distance when calling FindPeers
func WithDistance(dist uint8) discovery.Option {
	return func(opts *discovery.Options) error {
		opts.Other = make(map[interface{}]interface{})
		opts.Other["distance"] = dist
		return nil
	}
}
