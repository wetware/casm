package survey

import (
	"net"

	"github.com/libp2p/go-libp2p-core/discovery"
)

type DialFunc func(net.Addr) (net.PacketConn, error)
type ListenFunc func(net.Addr) (net.PacketConn, error)

// Config supplies options to the dependency-injection framework.
type Config struct {
	Dial   DialFunc
	Listen ListenFunc
}

func (c *Config) Apply(opt []Option) {
	for _, option := range withDefaults(opt) {
		option(c)
	}
}

type Option func(c *Config)

func WithDial(dial DialFunc) Option {
	if dial == nil {
		dial = func(addr net.Addr) (net.PacketConn, error) {
			udpAddr, err := net.ResolveUDPAddr(addr.Network(), addr.String())
			if err != nil {
				return nil, err
			}
			return net.ListenUDP(addr.Network(), udpAddr)
		}
	}

	return func(c *Config) {
		c.Dial = dial
	}
}

func WithListen(listen ListenFunc) Option {
	if listen == nil {
		listen = func(addr net.Addr) (net.PacketConn, error) {
			udpAddr, err := net.ResolveUDPAddr(addr.Network(), addr.String())
			if err != nil {
				return nil, err
			}
			return net.ListenMulticastUDP(addr.Network(), nil, udpAddr)
		}
	}

	return func(c *Config) {
		c.Listen = listen
	}
}

func withDefaults(opt []Option) []Option {
	return append([]Option{
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
