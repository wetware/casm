// Package bootutil provides utilities for parsing and instantiating boot services
package bootutil

import (
	"errors"
	"net"

	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/host"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"

	"github.com/wetware/casm/pkg/boot"
	"github.com/wetware/casm/pkg/boot/crawl"
	"github.com/wetware/casm/pkg/boot/socket"
	"github.com/wetware/casm/pkg/boot/survey"
)

// ErrUnknownBootProto is returned when the multiaddr passed
// to Parse does not contain a recognized boot protocol.
var ErrUnknownBootProto = errors.New("unknown boot protocol")

func DialString(h host.Host, s string, opt ...socket.Option) (discovery.Discoverer, error) {
	maddr, err := ma.NewMultiaddr(s)
	if err != nil {
		return nil, err
	}

	return Dial(h, maddr, opt...)
}

func Dial(h host.Host, maddr ma.Multiaddr, opt ...socket.Option) (discovery.Discoverer, error) {
	switch {
	case IsP2P(maddr):
		return boot.NewStaticAddrs(maddr)

	case IsCIDR(maddr):
		return DialCIDR(h, maddr, opt...)

	case IsMulticast(maddr):
		s, err := DialMulticast(h, maddr, opt...)
		if err != nil {
			return nil, err
		}

		if IsSurvey(maddr) {
			return survey.GradualSurveyor{Surveyor: s}, nil
		}

		return s, nil
	}

	return nil, ErrUnknownBootProto
}

func ListenString(h host.Host, s string, opt ...socket.Option) (discovery.Discovery, error) {
	maddr, err := ma.NewMultiaddr(s)
	if err != nil {
		return nil, err
	}

	return Listen(h, maddr, opt...)
}

func Listen(h host.Host, maddr ma.Multiaddr, opt ...socket.Option) (discovery.Discovery, error) {
	switch {
	case IsCIDR(maddr):
		return ListenCIDR(h, maddr, opt...)

	case IsMulticast(maddr):
		s, err := DialMulticast(h, maddr, opt...)
		if err != nil {
			return nil, err
		}

		if IsSurvey(maddr) {
			return survey.GradualSurveyor{Surveyor: s}, nil
		}

		return s, nil
	}

	return nil, ErrUnknownBootProto
}

func DialCIDR(h host.Host, maddr ma.Multiaddr, opt ...socket.Option) (*crawl.Crawler, error) {
	return newCrawler(h, maddr, func(m ma.Multiaddr) (net.PacketConn, error) {
		network, _, err := manet.DialArgs(maddr)
		if err != nil {
			return nil, err
		}

		return net.ListenPacket(network, ":0")
	}, opt)
}

func ListenCIDR(h host.Host, maddr ma.Multiaddr, opt ...socket.Option) (*crawl.Crawler, error) {
	return newCrawler(h, maddr, func(m ma.Multiaddr) (net.PacketConn, error) {
		network, address, err := manet.DialArgs(maddr)
		if err != nil {
			return nil, err
		}

		_, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}

		return net.ListenPacket(network, ":"+port)
	}, opt)
}

func DialMulticast(h host.Host, maddr ma.Multiaddr, opt ...socket.Option) (*survey.Surveyor, error) {
	group, ifi, err := survey.ResolveMulticast(maddr)
	if err != nil {
		return nil, err
	}

	conn, err := survey.JoinMulticastGroup("udp", ifi, group)
	if err != nil {
		return nil, err
	}

	return survey.New(h, conn, opt...), nil
}

func IsP2P(maddr ma.Multiaddr) bool {
	return hasBootProto(maddr, ma.P_P2P)
}

func IsCIDR(maddr ma.Multiaddr) bool {
	return hasBootProto(maddr, crawl.P_CIDR)
}

func IsMulticast(maddr ma.Multiaddr) bool {
	return hasBootProto(maddr, survey.P_MULTICAST)
}

func IsSurvey(maddr ma.Multiaddr) bool {
	return hasBootProto(maddr, survey.P_SURVEY)
}

func hasBootProto(maddr ma.Multiaddr, code int) bool {
	for _, p := range maddr.Protocols() {
		if p.Code == code {
			return true
		}
	}

	return false
}

type packetFunc func(ma.Multiaddr) (net.PacketConn, error)

func newCrawler(h host.Host, maddr ma.Multiaddr, newConn packetFunc, opt []socket.Option) (*crawl.Crawler, error) {
	s, err := crawl.ParseCIDR(maddr)
	if err != nil {
		return nil, err
	}

	conn, err := newConn(maddr)
	if err != nil {
		return nil, err
	}

	return crawl.New(h, conn, s, opt...), nil
}
