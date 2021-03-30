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
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	swarm "github.com/libp2p/go-libp2p-swarm"

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

	// EventJoined indicates a peer has joined the neighborhood.
	EventJoined Event = iota

	// EventLeft indicates a peer has left the neighborhood.
	EventLeft

	protectTag   = "mesh/neighborhood"
	defaultDepth = 7
)

// An Event represents a state transition in a neighborhood.
type Event uint8

func (e Event) Loggable() map[string]interface{} {
	return map[string]interface{}{
		"event": e.String(),
	}
}

func (e Event) String() string {
	switch e {
	case EventJoined:
		return "joined"
	case EventLeft:
		return "left"
	}

	panic(fmt.Sprintf("invalid event '%d'", e))
}

// Neighborhood is a local view of the overlay network.
type Neighborhood struct {
	ns   string
	proc goprocess.Process
	log  log.Logger

	mu    sync.RWMutex
	slots peer.IDSlice

	h  host.Host
	cb func(Event, peer.ID)

	graftable cond
}

// New neighborhood.
func New(h host.Host, opt ...Option) *Neighborhood {
	n := &Neighborhood{
		h:         h,
		graftable: make(cond, 1),
	}

	for _, option := range withDefaults(opt) {
		option(n)
	}

	h.Network().Notify(&network.NotifyBundle{
		DisconnectedF: leave(n),
	})

	for _, e := range []endpoint{
		join(n),
		sample(n),
	} {
		h.SetStreamHandler(e.Proto, e.NewHandler(n.log))
	}

	n.proc = goprocess.GoChild(h.Network().Process(), n.loop)
	n.proc.SetTeardown(n.teardown)

	return n
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
	n.h.Network().Process()
	n.mu.Lock()
	defer n.mu.Unlock()

	for _, proto := range []protocol.ID{
		JoinProto,
		SampleProto,
	} {
		n.h.RemoveStreamHandler(proto)
	}

	for _, id := range n.slots {
		n.callback(EventLeft, id)
	}
	n.slots = n.slots[:0]

	return nil
}

// callback handles neighborhood events.
// Callers must hold 'mu'.
func (n *Neighborhood) callback(e Event, id peer.ID) {
	switch e {
	case EventJoined:
		n.h.ConnManager().Protect(id, protectTag)
		n.cb(e, id)
		n.log.With(e).WithField("peer", id).Trace("peer joined")

	case EventLeft:
		n.h.ConnManager().Unprotect(id, protectTag)
		n.cb(e, id)
		n.log.With(e).WithField("peer", id).Trace("peer left")

	default:
		panic(fmt.Sprintf("invalid event '%d'", e))
	}

	switch {
	case len(n.slots) == 0:
		n.log.Warn("orphaned")
	case isGraftable(n.slots):
		n.graftable.Signal()
	}
}

// loop is responsible for maintaining the random overlay.
func (n *Neighborhood) loop(p goprocess.Process) {
	defer close(n.graftable)

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
			n.graftable.Signal()

		case <-n.graftable:
			if err := graft(n).Wait(); err != nil {
				n.log.WithError(err).Debug("graft failed")
			}

		case <-p.Closing():
			return

		}
	}
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

	// full?
	if len(ns) == 0 || isSaturated(ns) {
		return &any
	}

	r.Shuffle(cap(ns), func(i, j int) {
		ns[i], ns[j] = ns[j], ns[i]
	})

	ctx, cancel := context.WithTimeout(n.ctx(), time.Second*30)
	defer cancel()

	for i := 0; i < slots(n.slots); i++ {
		any.Go(n.sample(ctx, n.slots[i], defaultDepth))
	}

	return &any
}

// popRandom removes a random peer from the neighborhood.
// Callers must hold n.mu.
func popRandom(n *Neighborhood) {
	r.Shuffle(cap(n.slots), func(i, j int) {
		n.slots[i], n.slots[j] = n.slots[j], n.slots[i]
	})

	n.callback(EventLeft, n.slots[cap(n.slots)-1])
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

func leave(n *Neighborhood) func(network.Network, network.Conn) {
	return func(_ network.Network, c network.Conn) {
		n.mu.Lock()
		defer n.mu.Unlock()

		if rid := c.RemotePeer(); isNeighbor(n.slots, rid) {
			for i, id := range n.slots {
				if id == rid {
					n.callback(EventLeft, rid)
					n.slots[i] = n.slots[len(n.slots)-1] // move last element to i
					n.slots = n.slots[:len(n.slots)-1]   // pop last element
				}
			}
		}
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
			n.callback(EventJoined, s.Conn().RemotePeer())
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

type cond chan struct{}

func (c cond) Signal() {
	select {
	case c <- struct{}{}:
	default:
	}
}

// wait for a reader to close by blocking on a 'Read'
// call and discarding any data/error.
func wait(r io.Reader) {
	var buf [1]byte
	r.Read(buf[:])
}
