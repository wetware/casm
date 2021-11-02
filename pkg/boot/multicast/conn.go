package multicast

import (
	"context"
	"net"

	"capnproto.org/go/capnp/v3"
	"github.com/pkg/errors"
	"golang.org/x/net/ipv4"
)

type UDP struct {
	group *net.UDPAddr
	conn  *ipv4.PacketConn
}

func NewUDPConn(ctx context.Context, addr string, ifc *net.Interface) (*UDP, error) {
	ifc, err := ensureValidInterface(ifc)
	if err != nil {
		return nil, err
	}

	group, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}

	var lc net.ListenConfig
	conn, err := lc.ListenPacket(ctx, "udp", addr)
	if err != nil {
		return nil, err
	}

	pc := ipv4.NewPacketConn(conn)
	if err = pc.JoinGroup(ifc, group); err != nil {
		return nil, err
	}

	if err = pc.SetMulticastLoopback(true); err != nil {
		return nil, err
	}

	return &UDP{
		group: group,
		conn:  pc,
	}, err
}

func (u *UDP) Close() (err error) { return u.conn.Close() }

func (u *UDP) Scatter(ctx context.Context, msg *capnp.Message) (err error) {
	var b []byte
	if b, err = msg.MarshalPacked(); err != nil {
		return err
	}

	dl, _ := ctx.Deadline()
	if err = u.conn.SetWriteDeadline(dl); err == nil {
		_, err = u.conn.WriteTo(b, nil, u.group)
	}

	return err
}

func (u *UDP) Gather(ctx context.Context) (*capnp.Message, error) {
	b := make([]byte, maxDatagramSize) // TODO:  pool?

	dl, _ := ctx.Deadline()
	if err := u.conn.SetReadDeadline(dl); err != nil {
		return nil, err
	}

	n, _, _, err := u.conn.ReadFrom(b)
	if err != nil {
		return nil, err
	}

	return capnp.UnmarshalPacked(b[:n]) // TODO:  pool message
}

func ensureValidInterface(i *net.Interface) (*net.Interface, error) {
	if i == nil {
		return defaultInterface()
	}

	if validInterface(i) {
		return i, nil
	}

	return nil, errors.New("invalid interface")
}

func defaultInterface() (*net.Interface, error) {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	if ifs = selectValidInterfaces(ifs); len(ifs) == 0 {
		return nil, errors.New("multicast interface not found")
	}

	return &ifs[0], nil
}

func validInterface(i *net.Interface) bool {
	return len(selectValidInterfaces(ifSlice{*i})) > 0
}

func selectValidInterfaces(ifs ifSlice) ifSlice {
	return ifs.
		Filter(hasIPv4()).
		Filter(hasHardwareAddr()).
		Filter(hasFlags(net.FlagUp | net.FlagMulticast))
}

type ifSlice []net.Interface

func (s ifSlice) Filter(f func(*net.Interface) bool) ifSlice {
	ss := make(ifSlice, 0, len(s))
	for _, ifc := range s {
		if f(&ifc) {
			ss = append(ss, ifc)
		}
	}
	return ss
}

func hasIPv4() func(*net.Interface) bool {
	return func(i *net.Interface) bool {
		if as, err := i.Addrs(); err == nil {
			for _, a := range as {
				if a.(*net.IPNet).IP.To4() != nil {
					return true
				}
			}
		}

		return false
	}
}

func hasHardwareAddr() func(*net.Interface) bool {
	return func(i *net.Interface) bool {
		return len(i.HardwareAddr) > 0
	}
}

func hasFlags(fs net.Flags) func(*net.Interface) bool {
	return func(i *net.Interface) bool {
		return i.Flags&fs != 0
	}
}
