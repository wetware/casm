package mesh

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	swarm "github.com/libp2p/go-libp2p-swarm"
	"github.com/multiformats/go-multiaddr"

	"github.com/jbenet/goprocess"
	goprocessctx "github.com/jbenet/goprocess/context"
	"golang.org/x/sync/errgroup"

	"github.com/lthibault/jitterbug"
	"github.com/lthibault/log"
	syncutil "github.com/lthibault/util/sync"
)

var (
	// ErrClosed is returned from methods of Neighborhood when it
	// has left the overlay network.
	ErrClosed = errors.New("closed")

	// ErrNoPeers is a sentinel error used to signal that a reboot
	// has failed because there were no peers in the PeerStore.
	ErrNoPeers = errors.New("no peers")

	r = rand.New(rand.NewSource(time.Now().UnixNano()))
)

const (
	BaseProto   protocol.ID = "/casm/mesh"
	JoinProto               = BaseProto + "/join"
	SampleProto             = BaseProto + "/sample"

	protectTag = "mesh/neighborhood"
	streamTag  = "mesh/stream-active"

	defaultDepth = 7
)

// Neighborhood is a local view of the overlay network.
type Neighborhood struct {
	ns  string
	log log.Logger

	mu    sync.RWMutex
	slots peer.IDSlice

	h     host.Host
	proc  goprocess.Process
	event event.Emitter
}

// New neighborhood.
func New(h host.Host, opt ...Option) (*Neighborhood, error) {
	var (
		err     error
		n       = &Neighborhood{h: h}
		process goprocess.ProcessFunc
	)

	for _, option := range withDefaults(opt) {
		option(n)
	}

	if process, err = n.loop(); err != nil {
		return nil, err
	}

	n.proc = goprocess.GoChild(h.Network().Process(), process)
	defer n.proc.SetTeardown(n.teardown)

	h.Network().Notify((*notifiee)(n))

	for _, e := range []endpoint{
		join(n),
		sample(n),
	} {
		h.SetStreamHandler(e.Proto, e.NewHandler(n.log))
	}

	return n, nil
}

// Loggable representation of the neighborhood
func (n *Neighborhood) Loggable() map[string]interface{} {
	n.mu.RLock()
	defer n.mu.RUnlock()

	return map[string]interface{}{
		"type":  "casm.mesh.neighborhood",
		"id":    n.h.ID(),
		"k":     cap(n.slots),
		"conns": len(n.slots),
	}
}

// Neighbors are peers to which n is directly connected.
//
// It returns nil when n is not connected to the overlay network.
func (n *Neighborhood) Neighbors() peer.IDSlice {
	n.mu.RLock()
	defer n.mu.RUnlock()

	// Functions such as 'graft' rely on slice capacity to detect saturation.
	ns := make(peer.IDSlice, len(n.slots), cap(n.slots))
	copy(ns, n.slots)

	return ns
}

// Join an overlay network designated by 'ns', using 'd' to discover bootstrap peers.
func (n *Neighborhood) Join(ctx context.Context, d discovery.Discoverer, opt ...discovery.Option) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	peers, err := d.FindPeers(ctx, n.ns, opt...)
	if err != nil {
		return err
	}

	var brk syncutil.Breaker
	go func() {
		defer brk.Break()

		// This loop continues to be valid even after Wait() returns.
		// It will terminate when the 'peers' is exhausted.  Infinite
		// streams of bootstrap pears are valid.
		//
		// Note that this can be used in clever ways, for example by
		// having an external bootstrap service that periodically push
		// new peers to harden the mesh against partitions.
		for info := range peers {
			brk.Go(n.connect(ctx, info))
		}
	}()

	return brk.Wait()
}

// Close gracefully exits the overlay network without closing
// the underlying host.  It returns nil unless it was previously
// clsoed.
func (n *Neighborhood) Close() error { return n.proc.Close() }

func (n *Neighborhood) ctx() context.Context {
	return goprocessctx.OnClosingContext(n.proc)
}

func (n *Neighborhood) teardown() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	defer func() { n.slots = n.slots[:0] }()

	for _, proto := range []protocol.ID{
		JoinProto,
		SampleProto,
	} {
		n.h.RemoveStreamHandler(proto)
	}

	n.h.Network().StopNotify((*notifiee)(n))

	for _, id := range n.slots {
		_ = n.callback(EvtNeighborhoodChanged{
			Peer:  id,
			Event: EventLeft,
			State: StateClosed,
		})
	}

	return n.event.Close()
}

// callback handles neighborhood events.
// Callers must hold 'mu'.
func (n *Neighborhood) callback(e EvtNeighborhoodChanged) error {
	defer n.log.With(e).Trace(e)

	switch e.Event {
	case EventJoined:
		n.h.ConnManager().Protect(e.Peer, protectTag)

	case EventLeft:
		n.h.ConnManager().Unprotect(e.Peer, protectTag)

	default:
		panic(fmt.Sprintf("invalid event '%d'", e.Event))
	}

	return n.event.Emit(e)
}

// loop is responsible for maintaining the random overlay.
func (n *Neighborhood) loop() (goprocess.ProcessFunc, error) {
	var (
		err error
		bus = n.h.EventBus()
		sub event.Subscription
	)

	if n.event, err = bus.Emitter(new(EvtNeighborhoodChanged)); err != nil {
		return nil, err
	}

	if sub, err = bus.Subscribe(new(EvtNeighborhoodChanged)); err != nil {
		return nil, err
	}

	return func(p goprocess.Process) {
		defer sub.Close()

		/*
		 * We use a randomized ticker to avoid network storms
		 * in cases where a large number of peers are started
		 * in close succession.  See note below.
		 */
		ticker := jitterbug.New(time.Hour, jitterbug.Uniform{
			Source: r,
			Min:    time.Minute * 10,
		})
		defer ticker.Stop()

		/*
		 * Churn is generally not uniformly random, meaning that
		 * the overlay will tend to become less random over time.
		 * To mitigate this, we periodically 'prune' neighborhood
		 * connections by disconnecting from a random peer.  This
		 * is followed by a 'graft' operation, in which we sample
		 * the graph for new peers and connect to them.
		 *
		 * Additionally, neighbor disconnections signal that a new
		 * graft operation should be started.
		 *
		 * Note that because join operations must always succeed,
		 * joins on a full neighborhood will cause a random peer
		 * to be pruned, which can result in churn storms.  In
		 * order to mitigate this effect, we aim for k-1 neighbors
		 * and leave one slot open to "absorb" any churn.
		 */
		for {
			select {
			case <-ticker.C:
				prune(n)

			case <-p.Closing():
				return

			case v := <-sub.Out():
				switch e := v.(EvtNeighborhoodChanged); e.State {
				case StateEmpty:
					n.log.With(e).Warn("orphaned")

				case StatePartial:
					if err := graft(n).Wait(); err != nil {
						n.log.WithError(err).Debug("graft failed")
					}
				}
			}
		}
	}, nil
}

func prune(n *Neighborhood) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if isSaturated(n.slots) {
		popRandom(n)
	}
}

func graft(n *Neighborhood) *syncutil.Any {
	var (
		any syncutil.Any
		ns  = n.Neighbors()
	)

	if isGraftable(n.slots) {
		r.Shuffle(cap(ns), func(i, j int) {
			ns[i], ns[j] = ns[j], ns[i]
		})

		ctx, cancel := context.WithTimeout(n.ctx(), time.Second*30)
		defer cancel()

		for i := 0; i < slots(n.slots); i++ {
			any.Go(n.sample(ctx, n.slots[i], defaultDepth))
		}
	}

	return &any
}

// popRandom removes a random peer from the neighborhood.
// Callers must hold n.mu.
func popRandom(n *Neighborhood) {
	r.Shuffle(cap(n.slots), func(i, j int) {
		n.slots[i], n.slots[j] = n.slots[j], n.slots[i]
	})

	n.callback(EvtNeighborhoodChanged{
		Peer:  n.slots[cap(n.slots)-1],
		Event: EventLeft,
		State: state(n.slots[:cap(n.slots)-1]),
	})

	n.slots = n.slots[:cap(n.slots)-1] // pop
}

func (n *Neighborhood) connect(ctx context.Context, info peer.AddrInfo) func() error {
	return func() error {
		if err := n.h.Connect(ctx, info); err != nil {
			return err
		}

		s, err := n.h.NewStream(ctx, info.ID, JoinProto)
		if err != nil {
			if errors.Is(err, swarm.ErrDialToSelf) {
				return ErrNoPeers
			}
			return err
		}

		return join(n).Handle(s) // closes s
	}
}

func (n *Neighborhood) sample(ctx context.Context, id peer.ID, depth uint8) func() error {
	return func() error {
		s, err := n.h.NewStream(ctx, id, SampleProto)
		if err != nil {
			return err
		}
		defer s.Close()

		// ensure write deadline; default 30s.
		if t, ok := ctx.Deadline(); ok {
			s.SetWriteDeadline(t)
		} else {
			s.SetWriteDeadline(time.Now().Add(time.Second * 30))
		}

		if err = binary.Write(s, binary.BigEndian, depth-1); err != nil {
			return err
		}

		if err = s.CloseWrite(); err != nil {
			return err
		}

		// ensure read deadline; default 30s.
		if t, ok := ctx.Deadline(); ok {
			s.SetReadDeadline(t)
		} else {
			s.SetReadDeadline(time.Now().Add(time.Second * 30))
		}

		wait(s) // block until remote side closes
		return nil
	}
}

type endpoint struct {
	Proto  protocol.ID
	Handle func(s network.Stream) error
}

func (e endpoint) NewHandler(log log.Logger) network.StreamHandler {
	log = log.WithField("proto", e.Proto)

	return func(s network.Stream) {
		log = log.
			WithField("stream", s.ID()).
			WithField("peer", s.Conn().RemotePeer())

		log.Debug("stream accepted")
		defer log.Debug("stream closed")

		if err := e.Handle(s); err != nil {
			log.WithError(err).Debug("stream handler failed")
		}
	}
}

func join(n *Neighborhood) endpoint {
	return endpoint{
		Proto: JoinProto,
		Handle: func(s network.Stream) error {
			defer s.Close()

			n.mu.Lock()
			defer n.mu.Unlock()

			// duplicate connection?
			if isNeighbor(n.slots, s.Conn().RemotePeer()) {
				return nil
			}

			// neighborhood full?
			if isFull(n.slots) {
				popRandom(n)
			}

			// add to neighborhood
			n.slots = append(n.slots, s.Conn().RemotePeer()) // push
			n.callback(EvtNeighborhoodChanged{
				Peer:  s.Conn().RemotePeer(),
				Event: EventJoined,
				State: state(n.slots),
			})

			return nil
		},
	}
}

func sample(n *Neighborhood) endpoint {
	return endpoint{
		Proto: SampleProto,
		Handle: func(s network.Stream) error {
			defer s.Close()

			var depth uint8
			if err := binary.Read(s, binary.BigEndian, &depth); err != nil {
				return err
			}

			ctx, cancel := context.WithCancel(n.ctx())
			defer cancel()

			go func() {
				defer cancel()
				wait(s) // block until closed
			}()

			var g errgroup.Group

			// final destination?
			switch depth {
			case 0:
				info, err := peer.AddrInfoFromP2pAddr(s.Conn().RemoteMultiaddr())
				if err != nil {
					return err
				}

				g.Go(n.connect(ctx, *info))

			default:
				ns := n.Neighbors()
				r.Shuffle(len(ns), func(i, j int) {
					ns[i], ns[j] = ns[j], ns[i]
				})

				g.Go(n.sample(ctx, ns[0], depth-1))
			}

			return g.Wait()
		},
	}
}

func isNeighbor(ps peer.IDSlice, id peer.ID) bool {
	for _, n := range ps {
		if n == id {
			return true
		}
	}

	return false
}

func isFull(ps peer.IDSlice) bool {
	return len(ps) == cap(ps)
}

func isGraftable(ps peer.IDSlice) bool {
	if len(ps) > 0 && !isSaturated(ps) {
		return true
	}

	return false
}

func isSaturated(ps peer.IDSlice) bool {
	return len(ps) >= saturationPoint(ps)
}

// returns the saturation point (i.e. k-1 connections).
// the empty slot is designed to prevent churn.
func saturationPoint(ps peer.IDSlice) int {
	return cap(ps) - len(ps) - 1
}

// slots available
func slots(ps peer.IDSlice) int {
	return saturationPoint(ps) - len(ps)
}

// wait for a reader to close by blocking on a 'Read'
// call and discarding any data/error.
func wait(r io.Reader) {
	var buf [1]byte
	r.Read(buf[:])
}

type notifiee Neighborhood

var _ network.Notifiee = (*notifiee)(nil)

func (*notifiee) Listen(network.Network, multiaddr.Multiaddr)      {}
func (*notifiee) ListenClose(network.Network, multiaddr.Multiaddr) {}

func (n *notifiee) OpenedStream(_ network.Network, s network.Stream) {
	if s.Stat().Transient {
		return
	}

	(*Neighborhood)(n).h.ConnManager().UpsertTag(
		s.Conn().RemotePeer(),
		streamTag,
		func(i int) int { return i + 1 }) // incr
}

func (n *notifiee) ClosedStream(_ network.Network, s network.Stream) {
	if s.Stat().Transient {
		return
	}

	(*Neighborhood)(n).h.ConnManager().UpsertTag(
		s.Conn().RemotePeer(),
		streamTag,
		func(i int) int { return i - 1 }) // decr
}

func (*notifiee) Connected(network.Network, network.Conn) {}

func (n *notifiee) Disconnected(_ network.Network, c network.Conn) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if rid := c.RemotePeer(); isNeighbor(n.slots, rid) {
		for i, id := range n.slots {
			if id == rid {
				n.slots[i] = n.slots[len(n.slots)-1] // move last element to i
				n.slots = n.slots[:len(n.slots)-1]   // pop last element

				(*Neighborhood)(n).callback(EvtNeighborhoodChanged{
					Peer:  rid,
					Event: EventLeft,
					State: state(n.slots),
				})
			}
		}
	}
}
