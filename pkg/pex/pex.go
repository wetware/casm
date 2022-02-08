package pex

import (
	"context"
	"errors"
	"sync/atomic"

	"time"

	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/wetware/casm/pkg/boot"

	"github.com/lthibault/log"
)

const (
	mtu     = 2048             // maximum transmission unit => max capnp message size
	timeout = time.Second * 15 // Push-pull timeout
)

// GossipParam contains parameters for the PeX gossip algorithm.
type GossipParam struct {
	C int     // maximum View size
	S int     // swapping amount
	P int     // protection amount
	D float64 // retention decay probability
}

func (g GossipParam) mtu() int64 { return int64(g.C * mtu) }

func (g GossipParam) tail() func(View) View {
	return tail(g.P)
}

func (g GossipParam) head() func(View) View {
	return head((g.C / 2) - 1)
}

type StreamHandler interface {
	SetStreamHandlerMatch(protocol.ID, func(string) bool, network.StreamHandler)
	RemoveStreamHandler(pid protocol.ID)
}

// PeerExchange is a collection of passive views of various p2p clusters.
//
// For each namespace that is joined, PeerExchange maintains a bounded set
// of random peer addresses via its gossip protocol.  Peers are not directly
// monitored for liveness, so the addresses returned from FindPeers may be
// stale.  However, the PeX gossip-protocol guarantees that stale addresses
// are eventually expunged.
//
// Note that this is behavior reflects a fundamental trade-off in the design
// of the PeX protocol.  PeX strives to maintain a passive view of clusters
// that can be used to repair partitions and reconnect orphaned peers.  As a
// result, it must not immediately expunge unreachable peers from its records,
// else this would cause partitions to rapidly "forget" about each other.
//
// For the above reasons, we encourage users NOT to tune PeX parameters, as
// these have been carefully selected to work in a broad range of applications
// and micro-optimizations are likely to be counterproductive.
type PeerExchange struct {
	cq  <-chan struct{}
	log log.Logger

	add chan<- addGossiper
	get chan<- getGossiper
	gs  map[string]*gossiper

	disc    discovery.Discovery
	discOpt []discovery.Option
	store   rootStore
}

func New(ctx context.Context, h host.Host, opt ...Option) (*PeerExchange, error) {
	var (
		addq = make(chan addGossiper)
		getq = make(chan getGossiper)
	)

	var px = PeerExchange{
		cq:  ctx.Done(),
		add: addq,
		get: getq,
		gs:  make(map[string]*gossiper),
	}

	for _, option := range withDefaults(opt) {
		option(&px)
	}

	sub, err := h.EventBus().Subscribe(new(event.EvtLocalAddressesUpdated))
	if err != nil {
		return nil, err
	}

	// Sync the local record
	select {
	case v := <-sub.Out():
		px.store.Consume(v.(event.EvtLocalAddressesUpdated))

	case <-ctx.Done():
		defer sub.Close()
		return nil, ctx.Err()
	}

	// Update
	go func() {
		defer sub.Close()

		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case v := <-sub.Out():
				px.store.Consume(v.(event.EvtLocalAddressesUpdated))

			case t := <-ticker.C:
				for ns, g := range px.gs {
					if t.After(g.Deadline) {
						g.Cancel()
						g.UnregisterRPC(h)
						delete(px.gs, ns)
					}
				}

			case add := <-addq:
				g, ok := px.gs[add.NS]
				if !ok {
					g = add.NewGossiper(ctx, px.store.New(add.NS))
					g.RegisterRPC(px.log, h)
					px.gs[add.NS] = g
				}
				g.Deadline = time.Now().Add(add.Opt.Ttl)

			case get := <-getq:
				get.Res <- px.gs[get.NS]

			case <-ctx.Done():
				return
			}
		}
	}()

	return &px, nil
}

func (px *PeerExchange) Advertise(ctx context.Context, ns string, opt ...discovery.Option) (time.Duration, error) {
	var opts discovery.Options
	if err := opts.Apply(opt...); err != nil {
		return 0, err
	}

	ttl, err := px.disc.Advertise(ctx, ns, opt...)
	if err != nil {
		return 0, err
	}

	select {
	case px.add <- addGossiper{NS: ns, Opt: &opts}:
		return ttl, nil

	case <-ctx.Done():
		return 0, ctx.Err()

	case <-px.cq:
		return 0, errors.New("closed")
	}
}

func (px *PeerExchange) FindPeers(ctx context.Context, ns string, opt ...discovery.Option) (<-chan peer.AddrInfo, error) {
	var opts discovery.Options
	if err := opts.Apply(opt...); err != nil {
		return nil, err
	}

	var ch = make(chan *gossiper, 1) // TODO: pool
	select {
	case px.get <- getGossiper{NS: ns, Res: ch}:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-px.cq:
		return nil, errors.New("closed")
	}

	var g *gossiper
	select {
	case g = <-ch:
		if g == nil {
			return nil, errors.New("not found")
		}

	case <-ctx.Done():
		return nil, ctx.Err()
	case <-px.cq:
		return nil, errors.New("closed")
	}

	gs, err := g.LoadRecords()
	if err != nil {
		return nil, err
	}

	if len(gs) == 0 {
		return px.disc.FindPeers(ctx, ns, px.discOpt...)
	}

	sa := make(boot.StaticAddrs, len(gs))
	for i, r := range gs {
		sa[i].ID = r.PeerID
		sa[i].Addrs = r.Addrs
	}

	return sa.FindPeers(ctx, "")
}

type atomicRecord atomic.Value

func (rec *atomicRecord) Consume(e event.EvtLocalAddressesUpdated) {
	r, err := e.SignedPeerRecord.Record()
	if err != nil {
		panic(err)
	}

	(*atomic.Value)(rec).Store(r)
}

func (rec *atomicRecord) Load() *peer.PeerRecord {
	return (*atomic.Value)(rec).Load().(*peer.PeerRecord)
}

type addGossiper struct {
	NS  string
	Opt *discovery.Options
}

func (add addGossiper) NewGossiper(ctx context.Context, s gossipStore) *gossiper {
	return newGossiper(ctx, s, gossipParams(add.Opt))
}

type getGossiper struct {
	NS  string
	Res chan<- *gossiper // nil if not found
}
