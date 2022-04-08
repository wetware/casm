package boot

import (
	"errors"
	"io"
	"net"

	"github.com/lthibault/log"

	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/host"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	"github.com/wetware/casm/pkg/boot/crawl"
	"github.com/wetware/casm/pkg/boot/socket"
	"github.com/wetware/casm/pkg/boot/survey"
)

// ErrUnknownBootProto is returned when the multiaddr passed
// to Parse does not contain a recognized boot protocol.
var ErrUnknownBootProto = errors.New("unknown boot protocol")

type DiscoveryCloser interface {
	discovery.Discovery
	io.Closer
}

func Discover(log log.Logger, h host.Host, maddr ma.Multiaddr) (DiscoveryCloser, error) {
	switch {
	case crawler(maddr):
		s, err := crawl.ParseCIDR(maddr)
		if err != nil {
			return nil, err
		}

		conn, err := listenPacket(maddr)
		if err != nil {
			return nil, err
		}

		return crawl.New(h, conn, s, socket.WithLogger(log)), nil

	case multicast(maddr):
		group, ifi, err := survey.ResolveMulticast(maddr)
		if err != nil {
			return nil, err
		}

		conn, err := survey.JoinMulticastGroup("udp", ifi, group)
		if err != nil {
			return nil, err
		}

		s := survey.New(h, conn, socket.WithLogger(log))

		if !gradual(maddr) {
			return s, nil
		}

		return survey.GradualSurveyor{Surveyor: s}, nil
	}

	return nil, ErrUnknownBootProto
}

func listenPacket(maddr ma.Multiaddr) (net.PacketConn, error) {
	network, address, err := manet.DialArgs(maddr)
	if err != nil {
		return nil, err
	}

	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}

	return net.ListenPacket(network, ":"+port)
}

func crawler(maddr ma.Multiaddr) bool {
	return hasBootProto(maddr, crawl.P_CIDR)
}

func multicast(maddr ma.Multiaddr) bool {
	return hasBootProto(maddr, survey.P_MULTICAST)
}

func gradual(maddr ma.Multiaddr) bool {
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
