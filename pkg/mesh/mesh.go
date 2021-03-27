package mesh

import (
	"context"
	"errors"
	"math/rand"
	"sync"
	"sync/atomic"

	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"

	"zombiezen.com/go/capnproto2/rpc"
	"zombiezen.com/go/capnproto2/server"

	syncutil "github.com/lthibault/util/sync"

	"github.com/wetware/casm/internal/mesh"
)

var (
	// ErrDisconnected is returned from methods of Neighborhood when it
	// has left the overlay network.
	ErrDisconnected = errors.New("disconnected")
)

const (
	ProtocolID protocol.ID = "mesh"

	// EventJoined indicates a peer has joined the neighborhood.
	EventJoined Event = iota

	// EventLeft indicates a peer has left the neighborhood.
	EventLeft
)

// An Event represents a state transition in a neighborhood.
type Event uint8

// A Callback is invoked when a peer joins or leaves the
// neighborhood.
type Callback func(Event, peer.ID)

// Bootstrapper can provide bootstrap peers.
type Bootstrapper interface {
	Bootstrap(ctx context.Context) (<-chan peer.AddrInfo, error)
}

// Neighborhood is a local view of the overlay network.
type Neighborhood struct {
	mu *sync.Mutex
	cb Callback

	h host.Host

	es          map[peer.ID]*edge
	view        *atomic.Value
	k, overflow uint8

	once  *sync.Once
	cq    chan struct{}
	join  chan *edge
	leave chan peer.ID

	policy *server.Policy
}

// New neighborhood.
func New(h host.Host, opt ...Option) Neighborhood {
	n := Neighborhood{
		mu:    new(sync.Mutex),
		cb:    nopCallback,
		h:     h,
		once:  new(sync.Once),
		cq:    make(chan struct{}),
		join:  make(chan *edge),
		leave: make(chan peer.ID),
		view:  new(atomic.Value),
	}

	n.view.Store([]peer.ID{})
	h.SetStreamHandler(ProtocolID, n.handler)

	for _, option := range withDefaults(opt) {
		option(&n)
	}

	return n
}

// Join an overlay network, using the supplied bootstrapper.
//
// If Join returns nil, users must call Leave when finished
// to avoid leaking a goroutine.
func (n Neighborhood) Join(ctx context.Context, b Bootstrapper) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	peers, err := b.Bootstrap(ctx)
	if err != nil {
		return err
	}

	var brk syncutil.Breaker
	go func() {
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

// Leave the overlay network gracefully.
func (n Neighborhood) Leave(ctx context.Context) error {
	// TODO:  gracefully close connections

	return errors.New("Leave NOT IMPLEMENTED")
}

// Neighbors are peers to which n is directly connected.
//
// It returns nil when n is not connected to the overlay network.
func (n Neighborhood) Neighbors() []peer.ID {
	return n.view.Load().([]peer.ID)
}

// SetCallback assigns a callback to n that is called any time a neighbor
// joins or leaves the neighborhood.
func (n Neighborhood) SetCallback(f Callback) {
	if f == nil {
		f = nopCallback
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	n.cb = f
}

func (n Neighborhood) loop() {
	go func() {
		defer close(n.join)
		defer close(n.leave)

		for {
			select {
			case <-n.cq:
				return

			case e := <-n.join:
				// already connected?
				if _, ok := n.es[e.ID]; ok {
					_ = e.Close()
					continue
				}

				// too many edges?
				if len(n.es) > int(n.k+n.overflow) {
					popRandom(n.es)
				}

				n.es[e.ID] = e
				n.cb(EventJoined, e.ID)
				n.setView()

			case id := <-n.leave:
				// exists?
				if e, ok := n.es[id]; ok {
					_ = e.Close()
					delete(n.es, id)
					n.cb(EventLeft, id)
					n.setView()
				}
			}
		}
	}()
}

func (n Neighborhood) connect(ctx context.Context, info peer.AddrInfo) func() error {
	return func() error {
		if err := n.h.Connect(ctx, info); err != nil {
			return err
		}

		s, err := n.h.NewStream(ctx, info.ID, ProtocolID)
		if err != nil {
			return err
		}

		conn := rpc.NewConn(rpc.NewPackedStreamTransport(s), &rpc.Options{
			// ErrorReporter: /* TODO - logger */,
		})

		nc := &edge{
			ID:   s.Conn().RemotePeer(),
			Conn: conn,
			n:    mesh.Neighbor{Client: conn.Bootstrap(ctx)},
		}

		select {
		case <-n.cq:
			return ErrDisconnected
		case <-ctx.Done():
			return ctx.Err()
		case n.join <- nc:
			n.loop() // NOTE:  must be delayed until err guaranteed nil
			n.watch(nc)
		}

		return nil
	}
}

func (n Neighborhood) handler(s network.Stream) {
	nb := mesh.Neighbor_ServerToClient(neighbor{n}, n.policy)

	conn := rpc.NewConn(
		rpc.NewPackedStreamTransport(s),
		&rpc.Options{
			BootstrapClient: nb.Client,
			// ErrorReporter: /* TODO -  logger */,
		},
	)

	nc := &edge{
		ID:   s.Conn().RemotePeer(),
		Conn: conn,
		n:    nb,
	}

	select {
	case n.join <- nc:
		n.watch(nc)
	case <-n.cq:
	}
}

func (n Neighborhood) watch(e *edge) {
	go func() {
		<-e.Done()
		select {
		case n.leave <- e.ID:
		case <-n.cq:
			return
		}
	}()
}

func (n Neighborhood) setView() {
	v := make([]peer.ID, 0, len(n.es))
	for id := range n.es {
		v = append(v, id)
	}
	n.view.Store(v)
}

func nopCallback(Event, peer.ID) {}

type neighbor struct{ n Neighborhood }

func (n neighbor) Walk(ctx context.Context, walk mesh.Neighbor_walk) error {
	return errors.New("neighbor.Walk NOT IMPLEMENTED")
}

type edge struct {
	ID peer.ID
	*rpc.Conn
	n mesh.Neighbor
}

func popRandom(m map[peer.ID]*edge) {
	var i int
	x := rand.Intn(len(m) - 1)
	for id, e := range m {
		if i == x {
			e.Close()
			delete(m, id)
			break
		}

		i++
	}
}
