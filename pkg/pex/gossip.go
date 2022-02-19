package pex

import (
	"context"
	"io"
	"sync"
	"time"

	"capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/rpc"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/lthibault/log"
	syncutil "github.com/lthibault/util/sync"
	casm "github.com/wetware/casm/pkg"
	"github.com/wetware/casm/pkg/boot"
	protoutil "github.com/wetware/casm/pkg/util/proto"
)

// const (
// 	gossipTimeout = time.Second * 30
// 	maxMsgSize    = 2048 // max capnp message size
// )

type StreamHandler interface {
	SetStreamHandlerMatch(protocol.ID, func(string) bool, network.StreamHandler)
	RemoveStreamHandler(pid protocol.ID)
}

// GossipConfig contains parameters for the PeX gossip algorithm.
// Users SHOULD use the default settings.  The zero value is ready
// to use.
type GossipConfig struct {
	MaxView int     // maximum View size (default: 30)
	Swap    int     // swapping amount (default: 10)
	Protect int     // protection amount (default: 5)
	Decay   float64 // decay probability (default .005)

	// Tick defines the maximum interval separating two gossip
	// rounds.  If Tick == 0, a default value of 5min is used.
	//
	// Intervals are jittered in order to smooth out network load.
	// The actual tick duration is derived by uniformly sampling
	// the interval (Tick/2, Tick), resulting in a mean interval
	// of .75 * Tick.
	//
	// To avoid redundant gossip rounds, Tick SHOULD be at least
	// twice the value of Timeout.
	Tick time.Duration

	// Timeout specifies the maximum duration of a gossip round.
	// If Timeout == 0, a default value of of 30s is used.
	//
	// To avoid redundant gossip rounds, Timeout SHOULD be less
	// than half of Tick.
	Timeout time.Duration

	// MaxMsgSize specifies the maximum size of a single record over
	// the wire.  This is used to prevent amplification attacks.  If
	// MaxMsgSize == 0, a default value of 2048 is used.  This value
	// is quite generous, but moderate increases are reasonably safe.
	MaxMsgSize uint64
}

func (g GossipConfig) tail() func(View) View {
	return tail(g.Protect)
}

func (g GossipConfig) head() func(View) View {
	return head((g.MaxView / 2) - 1)
}

func (g GossipConfig) newDecoder(r io.Reader) *capnp.Decoder {
	dec := capnp.NewPackedDecoder(r)
	dec.MaxMessageSize = g.MaxMsgSize
	return dec
}

type gossiper struct {
	config GossipConfig
	store  gossipStore

	mvm mutexViewManager

	Stop func()
}

func (px *PeerExchange) newGossiper(ns string) *gossiper {
	var (
		ctx, cancel = context.WithCancel(px.ctx)
		log         = px.log.WithField("ns", ns)
		proto       = casm.Subprotocol(ns)
		protoPacked = casm.Subprotocol(ns, "packed")
		match       = casm.NewMatcher(ns)
		matchPacked = match.Then(protoutil.Exactly("packed"))
		config      = px.newParams(ns)
		store       = px.store.New(ns)
	)

	g := &gossiper{
		config: config,
		store:  store,
		mvm:    mutexViewManager{config: config, store: store},
		Stop: func() {
			cancel()
			px.h.RemoveStreamHandler(proto)
			px.h.RemoveStreamHandler(protoPacked)
		},
	}

	px.h.SetStreamHandlerMatch(
		proto,
		match,
		g.newHandler(ctx, log, rpc.NewStreamTransport))

	px.h.SetStreamHandlerMatch(
		protoPacked,
		matchPacked,
		g.newHandler(ctx, log, rpc.NewPackedStreamTransport))

	return g
}

func (g *gossiper) String() string { return g.store.ns }

func (g *gossiper) GetCachedPeers() (boot.StaticAddrs, error) {
	view, err := g.store.LoadView()
	if err != nil || view.Len() == 0 {
		return nil, err
	}

	info := make(boot.StaticAddrs, len(view))
	for i, rec := range view.Bind(shuffled()) {
		info[i].ID = rec.PeerID
		info[i].Addrs = rec.Addrs
	}

	return info, err
}

func (g *gossiper) PushPull(ctx context.Context, s network.Stream) error {
	var (
		j             syncutil.Join
		t, _          = ctx.Deadline()
		remote, local View
		err           error
	)

	if err := s.SetDeadline(t); err != nil {
		return err
	}

	local, err = g.mvm.getPushView()
	if err != nil {
		return err
	}

	// push
	j.Go(func() error {
		defer s.CloseWrite()

		buffer := local.
			//Bind(isNot(s.Conn().RemotePeer())).
			Bind(g.config.head()).
			Bind(appendLocal(g.store))

		enc := capnp.NewPackedEncoder(s)
		for _, gr := range buffer {
			if err = enc.Encode(gr.Message()); err != nil {
				break
			}
		}

		return err
	})

	// pull
	j.Go(func() error {
		defer s.CloseRead()

		dec := g.config.newDecoder(s)

		for {
			msg, err := dec.Decode()
			if err != nil {
				if err == io.EOF {
					break
				}
				return err
			}

			g := new(GossipRecord) // TODO(performance):  pool?
			if err = g.ReadMessage(msg); err != nil {
				return err
			}
			remote = append(remote, g)
		}
		return nil
	})

	if err = j.Wait(); err == nil {
		err = g.mvm.mergeAndStore(local, remote)
	}
	return err
}

func (g *gossiper) newHandler(ctx context.Context, log log.Logger, f transportFactory) network.StreamHandler {
	return func(s network.Stream) {
		slog := log.
			With(streamFields(s)).
			With(g.store)
		defer s.Close()

		ctx, cancel := context.WithTimeout(ctx, g.config.Timeout)
		defer cancel()

		log.Debug("handler started")
		defer func() { log.Debug("handler finished") }()

		if err := g.PushPull(ctx, s); err != nil {
			slog.WithError(err).Debug("peer exchange failed")
		}
	}
}

type transportFactory func(io.ReadWriteCloser) rpc.Transport

func (f transportFactory) NewTransport(rwc io.ReadWriteCloser) rpc.Transport { return f(rwc) }

func streamFields(s network.Stream) log.F {
	return log.F{
		"peer":   s.Conn().RemotePeer(),
		"conn":   s.Conn().ID(),
		"proto":  s.Protocol(),
		"stream": s.ID(),
	}
}
func min(n1, n2 int) int {
	if n1 <= n2 {
		return n1
	}
	return n2
}

func max(n1, n2 int) int {
	if n1 <= n2 {
		return n2
	}
	return n1
}

type mutexViewManager struct {
	mu sync.Mutex

	store  gossipStore
	config GossipConfig
}

func (mvm *mutexViewManager) getPushView() (local View, err error) {
	mvm.mu.Lock()
	defer mvm.mu.Unlock()

	local, err = mvm.store.LoadView()
	if err != nil {
		return
	}

	local = local.Bind(sorted())
	oldest := local.Bind(mvm.config.tail())

	local = append(
		local.
			Bind(head(len(local)-len(oldest))).
			Bind(shuffled()),
		oldest...)
	return
}

func (mvm *mutexViewManager) mergeAndStore(local, remote View) error {
	if err := remote.Validate(); err != nil {
		return err
	}

	// Remove duplicates and combine local and remote records
	newLocal := local.
		Bind(merged(remote)).
		Bind(isNot(mvm.store.Record().PeerID))

	// Apply swapping
	s := min(mvm.config.Swap, max(len(newLocal)-mvm.config.MaxView, 0))
	newLocal = newLocal.
		Bind(tail(len(newLocal) - s)).
		Bind(sorted())

	// Apply retention
	r := min(min(mvm.config.Protect, mvm.config.MaxView), len(newLocal))
	maxDecay := min(r, max(len(newLocal)-mvm.config.MaxView, 0))
	oldest := newLocal.Bind(tail(r)).Bind(decay(mvm.config.Decay, maxDecay))

	//Apply random eviction
	c := mvm.config.MaxView - len(oldest)
	newLocal = newLocal.
		Bind(head(max(len(newLocal)-r, 0))).
		Bind(shuffled()).
		Bind(head(c))

	// Merge with oldest nodes
	newLocal = newLocal.
		Bind(merged(oldest))

	newLocal.incrHops()

	return mvm.store.StoreRecords(local, newLocal)
}
