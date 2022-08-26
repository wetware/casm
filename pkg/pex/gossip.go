package pex

import (
	"context"
	"io"
	"sync"
	"time"

	"capnproto.org/go/capnp/v3"
	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/lthibault/log"
	"github.com/pierrec/lz4/v4"

	ctxutil "github.com/lthibault/util/ctx"
	syncutil "github.com/lthibault/util/sync"
	casm "github.com/wetware/casm/pkg"
	"github.com/wetware/casm/pkg/boot"
)

func proto(ns string) protocol.ID {
	return casm.Subprotocol(ns, "pex")
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

type gossiper struct {
	config       GossipConfig
	store        gossipStore
	peersUpdated event.Emitter

	mu sync.Mutex

	Stop func()
}

func (px *PeerExchange) newGossiper(ns string) *gossiper {
	var (
		ctx, cancel = context.WithCancel(ctxutil.C(px.done))
		log         = px.log.WithField("ns", ns)
	)

	g := &gossiper{
		config:       px.newParams(ns),
		store:        px.store.New(ns),
		peersUpdated: px.peersUpdated,
		Stop: func() {
			cancel()
			px.h.RemoveStreamHandler(proto(ns))
		},
	}

	px.h.SetStreamHandler(proto(ns), g.newHandler(ctx, log))

	return g
}

func (g *gossiper) String() string { return g.store.ns }

func (g *gossiper) GetCachedPeers(ctx context.Context) (boot.StaticAddrs, error) {
	view, err := g.store.LoadView(ctx)
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

func (g *gossiper) OpenStream(ctx context.Context, h host.Host, info peer.AddrInfo) (network.Stream, error) {
	if err := h.Connect(ctx, info); err != nil {
		return nil, err
	}

	return h.NewStream(ctx, info.ID, proto(g.String()))
}

func (g *gossiper) PushPull(ctx context.Context, s network.Stream) error {
	round := &gossipRound{g: g}
	return round.Do(ctx, s)
}

func (g *gossiper) newHandler(ctx context.Context, log log.Logger) network.StreamHandler {
	return func(s network.Stream) {
		slog := log.
			With(streamFields(s)).
			With(g.store)
		defer s.Close()

		ctx, cancel := context.WithTimeout(ctx, g.config.Timeout)
		defer cancel()

		if err := g.PushPull(ctx, s); err != nil {
			slog.WithError(err).Debug("peer exchange failed")
		}
	}
}

func streamFields(s network.Stream) log.F {
	return log.F{
		"peer":   s.Conn().RemotePeer(),
		"conn":   s.Conn().ID(),
		"proto":  s.Protocol(),
		"stream": s.ID(),
	}
}

func (g *gossiper) mutexGetPushView(ctx context.Context) (local View, err error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	local, err = g.store.LoadView(ctx)
	if err != nil {
		return
	}

	local = local.Bind(sorted())
	oldest := local.Bind(tail(g.config.Protect))

	local = append(
		local.
			Bind(head(len(local)-len(oldest))).
			Bind(shuffled()),
		oldest...)
	return
}

func (g *gossiper) mutexMergeAndStore(ctx context.Context, r *gossipRound) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	newLocal, err := g.merge(r.local, r.remote)
	if err != nil {
		return err
	}

	if err = g.store.StoreRecords(ctx, r.local, newLocal); err == nil {
		g.peersUpdated.Emit(EvtPeersUpdated(newLocal.PeerRecords()))
	}

	return err
}

func (g *gossiper) merge(local, remote View) (View, error) {
	if err := remote.Validate(); err != nil {
		return nil, err
	}

	// Remove duplicates and combine local and remote records
	newLocal := local.
		Bind(merged(remote)).
		Bind(isNot(g.store.Record().PeerID))

	// Apply swapping
	s := min(g.config.Swap, max(len(newLocal)-g.config.MaxView, 0))
	newLocal = newLocal.
		Bind(tail(len(newLocal) - s)).
		Bind(sorted())

	// Apply retention
	r := min(min(g.config.Protect, g.config.MaxView), len(newLocal))
	maxDecay := min(r, max(len(newLocal)-g.config.MaxView, 0))
	oldest := newLocal.Bind(tail(r)).Bind(decay(g.config.Decay, maxDecay))

	// Apply random eviction
	c := g.config.MaxView - len(oldest)
	newLocal = newLocal.
		Bind(head(max(len(newLocal)-r, 0))).
		Bind(shuffled()).
		Bind(head(c))

	// Merge with oldest nodes
	newLocal = newLocal.
		Bind(merged(oldest))

	newLocal.incrHops()

	return newLocal, nil
}

// gossipRound encapsulates temporary state for a single gossip round.
type gossipRound struct {
	j             syncutil.Join
	g             *gossiper
	remote, local View
}

func (r *gossipRound) Do(ctx context.Context, s network.Stream) error {
	if err := r.prepare(ctx, s); err != nil {
		return err
	}

	if err := r.pushpull(s); err != nil {
		return err
	}

	return r.mergeAndStore(ctx)
}

func (r *gossipRound) prepare(ctx context.Context, s network.Stream) (err error) {
	t, _ := ctx.Deadline()
	if err = s.SetDeadline(t); err == nil {
		r.local, err = r.g.mutexGetPushView(ctx)
	}

	return
}

func (r *gossipRound) pushpull(s network.Stream) error {
	r.j.Go(r.push(s))
	r.j.Go(r.pull(s))
	return r.j.Wait()
}

func (r *gossipRound) push(s network.Stream) func() error {
	return func() error {
		defer s.CloseWrite()

		enc := encoder{W: s}
		defer enc.Close()

		buffer := r.local.
			Bind(head((r.g.config.MaxView / 2) - 1)).
			Bind(appendLocal(r.g.store))

		for _, gr := range buffer {
			if err := enc.Encode(gr.Message()); err != nil {
				return err
			}
		}

		return enc.Flush()
	}
}

func (r *gossipRound) pull(s network.Stream) func() error {
	return func() error {
		defer s.CloseRead()

		dec := decoder{R: s, MaxMsgSize: r.g.config.MaxMsgSize}

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
			r.remote = append(r.remote, g)
		}

		return nil
	}
}

func (r *gossipRound) mergeAndStore(ctx context.Context) error {
	return r.g.mutexMergeAndStore(ctx, r)
}

type encoder struct {
	W io.Writer
	w *lz4.Writer
	e *capnp.Encoder
}

func (enc *encoder) Encode(m *capnp.Message) error {
	if enc.e == nil {
		enc.w = lz4.NewWriter(enc.W)
		enc.e = capnp.NewPackedEncoder(enc.w)
	}

	return enc.e.Encode(m)
}

func (enc *encoder) Flush() (err error) {
	if enc.w != nil {
		err = enc.w.Flush()
	}

	return
}

func (enc *encoder) Close() (err error) {
	if enc.w != nil {
		err = enc.w.Close()
	}

	return
}

type decoder struct {
	R          io.Reader
	MaxMsgSize uint64
	d          *capnp.Decoder
}

func (dec *decoder) Decode() (*capnp.Message, error) {
	if dec.d == nil {
		dec.d = capnp.NewPackedDecoder(lz4.NewReader(dec.R))
		dec.d.MaxMessageSize = dec.MaxMsgSize
	}

	return dec.d.Decode()
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
