package survey

import "net"

type DialFunc func(net.Addr) (net.PacketConn, error)

type ListenFunc func(net.Addr) (net.PacketConn, error)

func (dfun DialFunc) Dial(addr net.Addr) (net.PacketConn, error) {
	if dfun == nil {
		return DialUdp(addr)
	}
	return dfun(addr)
}

func (lfun ListenFunc) Listen(addr net.Addr) (net.PacketConn, error) {
	if lfun == nil {
		return ListenMulticastUdp(addr)
	}
	return lfun(addr)
}

type MulticastTransport struct {
	DialFunc
	ListenFunc
}

func DialUdp(addr net.Addr) (net.PacketConn, error) {
	if addr != nil {
		return net.ListenUDP(addr.Network(), nil)
	}
	return net.ListenUDP("udp", nil)
}

func ListenMulticastUdp(addr net.Addr) (net.PacketConn, error) {
	var (
		udpAddr *net.UDPAddr
		err     error
	)

	udpAddr, err = net.ResolveUDPAddr(addr.Network(), addr.String())
	if err != nil {
		return nil, err
	}
	return net.ListenMulticastUDP(addr.Network(), nil, udpAddr)
}
