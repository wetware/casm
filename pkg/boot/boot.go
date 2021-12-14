package boot

import (
	"context"
	"net"
	"time"

	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/wetware/casm/pkg/packet"
)

type Strategy struct {
	OnAdvertise func(context.Context, string, ...discovery.Option) (time.Duration, error)
	OnDiscover  func(context.Context, string, ...discovery.Option) (<-chan peer.AddrInfo, error)
}

func (s Strategy) Advertise(ctx context.Context, ns string, opt ...discovery.Option) (time.Duration, error) {
	if s.OnAdvertise == nil {
		s.OnAdvertise = StaticAddrs(nil).Advertise
	}

	return s.OnAdvertise(ctx, ns, opt...)
}

func (s Strategy) FindPeers(ctx context.Context, ns string, opt ...discovery.Option) (<-chan peer.AddrInfo, error) {
	if s.OnDiscover == nil {
		s.OnDiscover = StaticAddrs(nil).FindPeers
	}

	return s.OnDiscover(ctx, ns, opt...)
}

// Knock on a port and await an answer.
func Knock(ctx context.Context, r packet.RoundTripper, conn net.PacketConn) (info peer.AddrInfo, err error) {
	if err := bind(ctx, conn); err != nil {
		return peer.AddrInfo{}, err
	}

	var rec peer.PeerRecord
	_, err = r.RoundTrip(conn, &rec)
	return peer.AddrInfo{
		ID:    rec.PeerID,
		Addrs: rec.Addrs,
	}, err
}

// Answer knocks with a record.
func Answer(ctx context.Context, h packet.Handler, conn net.PacketConn) error {
	var buf [packet.MaxMsgSize]byte
	n, addr, err := conn.ReadFrom(buf[:])
	if err != nil {
		return err
	}

	n, err = h.ServePacket(buf[:], n)
	if err != nil {
		return err
	}

	_, err = conn.WriteTo(buf[:n], addr)
	return err
}

func bind(ctx context.Context, conn net.PacketConn) error {
	if t, ok := ctx.Deadline(); ok {
		return conn.SetDeadline(t)
	}

	return conn.SetDeadline(time.Now().Add(time.Millisecond))
}
