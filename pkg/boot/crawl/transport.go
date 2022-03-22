package crawl

import "net"

type DialFunc func(net.Addr) (net.PacketConn, error)

type ListenFunc func(net.Addr) (net.PacketConn, error)

func (dfun DialFunc) Dial(addr net.Addr) (net.PacketConn, error) {
	if dfun == nil {
		return UdpDial(addr)
	}
	return dfun(addr)
}

func (lfun ListenFunc) Listen(addr net.Addr) (net.PacketConn, error) {
	if lfun == nil {
		return UdpListen(addr)
	}
	return lfun(addr)
}

type Transport struct {
	DialFunc
	ListenFunc
}

func UdpDial(addr net.Addr) (net.PacketConn, error) {
	if addr != nil {
		return net.ListenUDP(addr.Network(), nil)
	}
	return net.ListenUDP("udp", nil)
}

func UdpListen(addr net.Addr) (net.PacketConn, error) {
	var (
		udpAddr *net.UDPAddr
		err     error
	)

	if addr != nil {
		udpAddr, err = net.ResolveUDPAddr(addr.Network(), addr.String())
		if err != nil {
			return nil, err
		}
		return net.ListenUDP(addr.Network(), udpAddr)
	}
	return net.ListenUDP("udp", nil)
}
