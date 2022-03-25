package boot

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"

	"github.com/lthibault/log"

	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/host"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	"github.com/wetware/casm/pkg/boot/crawl"
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
		s, err := strategy(maddr)
		if err != nil {
			return nil, err
		}

		network, addr, err := manet.DialArgs(maddr)
		if err != nil {
			return nil, err
		}

		conn, err := net.ListenPacket(network, addr)
		if err != nil {
			return nil, err
		}

		return crawl.New(h, conn,
			crawl.WithLogger(log),
			crawl.WithStrategy(s)), nil

	case multicast(maddr):
		group, ifi, err := survey.ResolveMulticast(maddr)
		if err != nil {
			return nil, err
		}

		conn, err := survey.JoinMulticastGroup("udp", ifi, group)
		if err != nil {
			return nil, err
		}

		s := survey.New(h, conn, survey.WithLogger(log))

		if !gradual(maddr) {
			return s, nil
		}

		return survey.GradualSurveyor{Surveyor: s}, nil
	}

	return nil, ErrUnknownBootProto
}

func strategy(maddr ma.Multiaddr) (crawl.Strategy, error) {
	_, addr, err := manet.DialArgs(maddr)
	if err != nil {
		return nil, err
	}

	ap, err := netip.ParseAddrPort(addr)
	if err != nil {
		return nil, err
	}

	cidr, err := maddr.ValueForProtocol(crawl.P_CIDR)
	if err != nil {
		return nil, err
	}

	r, err := crawl.NewCIDR(
		fmt.Sprintf("%s/%s", ap.Addr(), cidr),
		int(ap.Port()))

	return func() crawl.Range { return r }, err
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
