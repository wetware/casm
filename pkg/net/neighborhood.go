package net

import (
	"context"

	"github.com/jbenet/goprocess"
	ctxutil "github.com/lthibault/util/ctx"
	"github.com/multiformats/go-multiaddr"

	"github.com/libp2p/go-libp2p-core/connmgr"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
)

const (
	protectTag = "casm.net/neighborhood"
	streamTag  = "casm.net/stream-active"

	bufSize = 32
)

type neighborhood struct {
	log Logger

	ns    Slots
	state chan op

	cm   connmgr.ConnManager
	proc goprocess.Process
}

func (n *neighborhood) Lease(ctx context.Context, peer peer.AddrInfo) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case n.state <- lease(peer):
		return nil
	}
}

func (n *neighborhood) Evict(ctx context.Context, peer peer.AddrInfo) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case n.state <- evict(peer):
		return nil
	}
}

func (n *neighborhood) loop() (goprocess.ProcessFunc, <-chan *EvtNetState) {
	n.state = make(chan op, bufSize)
	ch := make(chan *EvtNetState, bufSize/2)

	return func(p goprocess.Process) {
		defer n.drainAndCloseStateChan()
		defer close(ch)

		for {
			select {
			case <-p.Closing():
				return
			case s := <-n.state:
				if ev, ok := n.handle(s.AddrInfo(), s.Event()); ok {
					select {
					case ch <- ev:
					case <-p.Closing():
					}
				}
			}
		}
	}, ch
}

func (n *neighborhood) handle(info peer.AddrInfo, ev Event) (*EvtNetState, bool) {
	switch ev {
	case EventJoined:
		if !n.ns.insert(info) {
			return nil, false
		}
		n.cm.Protect(info.ID, protectTag)

	case EventLeft:
		if !n.ns.delete(info) {
			return nil, false
		}
		n.cm.Unprotect(info.ID, protectTag)
	}

	return &EvtNetState{
		Peer:  info.ID,
		Event: ev,
		Slots: n.ns.View(),
	}, true
}

func (n *neighborhood) drainAndCloseStateChan() {
	for {
		select {
		case <-n.state:
		default:
			close(n.state)
			return
		}
	}
}

func (n *neighborhood) ctx() context.Context {
	return ctxutil.FromChan(n.proc.Closing())
}

func (n *neighborhood) Listen(network.Network, multiaddr.Multiaddr)      {}
func (n *neighborhood) ListenClose(network.Network, multiaddr.Multiaddr) {}

func (n *neighborhood) Connected(network.Network, network.Conn) {}

func (n *neighborhood) Disconnected(_ network.Network, c network.Conn) {
	info, err := peer.AddrInfoFromP2pAddr(c.RemoteMultiaddr())
	if err != nil {
		n.log.WithError(err).WithConn(c).Error("invalid multiaddr")
		return
	}

	if err := n.Evict(n.ctx(), *info); err != nil {
		n.log.WithError(err).WithConn(c).Error("failed to evict disconnected peer")
	}
}

func (n *neighborhood) OpenedStream(_ network.Network, s network.Stream) {
	info, err := peer.AddrInfoFromP2pAddr(s.Conn().RemoteMultiaddr())
	if err != nil {
		n.log.WithError(err).WithStream(s).Error("invalid multiaddr")
		return
	}

	if s.Protocol() == ProtoJoin {
		err := n.Lease(n.ctx(), *info)
		if err != nil {
			n.log.WithError(err).WithStream(s).Error("failed to lease.")
		}
		return
	}

	n.cm.UpsertTag(
		info.ID,
		streamTag,
		func(i int) int { return i + 1 }) // incr
}

func (n *neighborhood) ClosedStream(_ network.Network, s network.Stream) {
	if s.Protocol() != ProtoJoin {
		return
	}

	n.cm.UpsertTag(
		s.Conn().RemotePeer(),
		streamTag,
		func(i int) int { return i - 1 }) // decr
}

type Slots []peer.AddrInfo

func (s Slots) State() State {
	if len(s) == 0 {
		return StateDisconnected
	}

	return StateConnected
}

func (s Slots) Contains(id peer.ID) (ok bool) {
	for _, info := range s {
		if ok = info.ID == id; ok {
			break
		}
	}

	return
}

func (s Slots) Shuffle() []peer.AddrInfo {
	ns := s.View()
	r.Shuffle(len(ns), func(i, j int) {
		ns[i], ns[j] = ns[j], ns[i]
	})
	return ns
}

func (s Slots) View() Slots {
	ss := make(Slots, len(s))
	copy(ss, s)
	return ss
}

func (s *Slots) delete(x peer.AddrInfo) (ok bool) {
	ss := (*s)[:0]
	for _, info := range *s {
		if info.ID != x.ID {
			ss = append(ss, x)
		}
		ok = true
	}

	*s = ss
	return
}

func (s *Slots) insert(x peer.AddrInfo) (ok bool) {
	if ok = !s.Contains(x.ID); ok {
		*s = append(*s, x)
	}
	return
}

type op interface {
	AddrInfo() peer.AddrInfo
	Event() Event
}

type lease peer.AddrInfo

func (lease) Event() Event               { return EventJoined }
func (op lease) AddrInfo() peer.AddrInfo { return peer.AddrInfo(op) }

type evict peer.AddrInfo

func (evict) Event() Event               { return EventLeft }
func (op evict) AddrInfo() peer.AddrInfo { return peer.AddrInfo(op) }
