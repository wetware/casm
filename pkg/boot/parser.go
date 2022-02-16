package boot

import (
	"errors"
	"fmt"
	"net"
	"reflect"
	"strconv"

	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/host"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	"github.com/wetware/casm/pkg/boot/crawl"
	"github.com/wetware/casm/pkg/boot/survey"
)

const (
	P_CIDR = iota + 100
	P_SURVEY
	P_GRADUAL
)

var (
	// ErrUnknownBootProto is returned when the multiaddr passed
	// to Parse does not contain a recognized boot protocol.
	ErrUnknownBootProto = errors.New("unknown boot protocol")

	// ErrCIDROverflow is returned when a CIDR block is too large.
	ErrCIDROverflow = errors.New("CIDR overflow")

	// ErrDistanceOverflow is returned when a peer-distance is too large.
	ErrDistanceOverflow = errors.New("distance overflow")
)

func init() {
	for _, p := range []ma.Protocol{
		{
			Name:       "cidr",
			Code:       P_CIDR,
			VCode:      ma.CodeToVarint(P_CIDR),
			Size:       8,
			Transcoder: TranscoderCIDR{},
		},
		{
			Name:  "survey",
			Code:  P_SURVEY,
			VCode: ma.CodeToVarint(P_SURVEY),
		},
		{
			Name:  "gradual",
			Code:  P_GRADUAL,
			VCode: ma.CodeToVarint(P_GRADUAL),
		},
	} {
		if err := ma.AddProtocol(p); err != nil {
			panic(err)
		}
	}
}

func Parse(h host.Host, maddr ma.Multiaddr) (discovery.Discoverer, error) {
	a, err := parseLayer4(maddr)
	if err != nil {
		return nil, err
	}

	addr, err := manet.ToNetAddr(a)
	if err != nil {
		return nil, err
	}

	switch {
	// CRAWL
	case hasBootProto(maddr, P_CIDR):
		cidr, err := parseCIDR(maddr)
		if err != nil {
			return nil, err
		}

		return newCrawler(addr, cidr)

	// SURVEY
	case hasBootProto(maddr, P_SURVEY):
		s, err := survey.New(h, addr)
		if err != nil || !hasBootProto(maddr, P_GRADUAL) {
			return s, err
		}

		return survey.GradualSurveyor{
			Surveyor: s,
		}, nil
	}

	return nil, ErrUnknownBootProto
}

// parseLayer4 fetches the first two components from a multiaddr, which
// are expected to IPs and TCP/UDP repsectively, and joins them into a
// single multiaddress.
func parseLayer4(maddr ma.Multiaddr) (ma.Multiaddr, error) {
	var cs []ma.Multiaddr
	ma.ForEach(maddr, func(c ma.Component) bool {
		cs = append(cs, &c)
		return len(cs) < 2
	})

	if len(cs) < 2 {
		return nil, errors.New("not enough components")
	}

	switch cs[0].(*ma.Component).Protocol().Code {
	case ma.P_IP4, ma.P_IP6:
	default:
		return nil, errors.New("no ip protocol")
	}

	switch cs[1].(*ma.Component).Protocol().Code {
	case ma.P_TCP, ma.P_UDP:
	default:
		return nil, errors.New("no transport protocol")
	}

	return ma.Join(cs[0], cs[1]), nil
}

func newCrawler(addr net.Addr, cidr int) (c crawl.Crawler, err error) {
	switch a := addr.(type) {
	case *net.TCPAddr:
		c.Strategy = &crawl.ScanSubnet{
			Net:  a.Network(),
			Port: a.Port,
			CIDR: fmt.Sprintf("%v/%v", a.IP, cidr), // e.g. '10.0.1.0/24'
		}

	case *net.UDPAddr:
		err = fmt.Errorf("UDP crawler NOT IMPLEMENTED") // TODO

	default:
		err = fmt.Errorf("crawler not implemented for %s", reflect.TypeOf(addr))
	}

	return
}

func hasBootProto(maddr ma.Multiaddr, code int) bool {
	for _, p := range maddr.Protocols() {
		if p.Code == code {
			return true
		}
	}

	return false
}

func parseCIDR(maddr ma.Multiaddr) (cidr int, err error) {
	ma.ForEach(maddr, func(c ma.Component) bool {
		if c.Protocol().Code != P_CIDR {
			return true // continue ...
		}

		b := c.RawValue()
		if err = c.Protocol().Transcoder.ValidateBytes(b); err == nil {
			cidr = int(c.RawValue()[0])
		}

		return false
	})

	return
}

// TranscoderCIDR decodes a uint8 CIDR block
type TranscoderCIDR struct{}

func (ct TranscoderCIDR) StringToBytes(cidrBlock string) ([]byte, error) {
	num, err := strconv.ParseUint(cidrBlock, 10, 8)
	if err != nil {
		return nil, err
	}

	if num > 128 {
		return nil, ErrCIDROverflow
	}

	return []byte{uint8(num)}, err
}

func (ct TranscoderCIDR) BytesToString(b []byte) (string, error) {
	if len(b) > 1 || b[0] > 128 {
		return "", ErrCIDROverflow
	}

	return strconv.FormatUint(uint64(b[0]), 10), nil
}

func (ct TranscoderCIDR) ValidateBytes(b []byte) error {
	if uint8(b[0]) > 128 { // 128 is maximum CIDR block for IPv6
		return ErrCIDROverflow
	}

	return nil
}
