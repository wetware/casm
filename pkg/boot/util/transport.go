package util

import "net"

type DialFunc func(net.Addr) (net.PacketConn, error)

type ListenFunc func(net.Addr) (net.PacketConn, error)

type Transport struct {
	Dial   DialFunc
	Listen ListenFunc
}

var (
	MulticastTransport = Transport{Dial: UdpDial, Listen: MulticastListen}
	UdpTransport       = Transport{Dial: UdpDial, Listen: UdpListen}
)

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

func MulticastListen(addr net.Addr) (net.PacketConn, error) {
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
