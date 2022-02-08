package pex

import (
	"context"
	"io"
	"time"

	"capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/rpc"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/lthibault/log"
	syncutil "github.com/lthibault/util/sync"
	casm "github.com/wetware/casm/pkg"
	protoutil "github.com/wetware/casm/pkg/util/proto"
)

type gossiper struct {
	Deadline time.Time
	param    GossipParam
	gossipStore
}

func newGossiper(gs gossipStore, ps GossipParam) *gossiper {
	return &gossiper{
		param:       ps,
		gossipStore: gs,
	}
}

func (g *gossiper) RegisterRPC(log log.Logger, h StreamHandler) {
	var (
		match       = casm.NewMatcher(g.String())
		matchPacked = match.Then(protoutil.Exactly("packed"))
	)

	h.SetStreamHandlerMatch(
		casm.Subprotocol(g.String()),
		match,
		g.newHandler(log, rpc.NewStreamTransport))

	h.SetStreamHandlerMatch(
		casm.Subprotocol(g.String(), "packed"),
		matchPacked,
		g.newHandler(log, rpc.NewPackedStreamTransport))
}

func (g *gossiper) UnregisterRPC(h StreamHandler) {
	h.RemoveStreamHandler(casm.Subprotocol(g.String()))
	h.RemoveStreamHandler(casm.Subprotocol(g.String(), "packed"))
}

func (g *gossiper) newHandler(log log.Logger, f transportFactory) network.StreamHandler {
	return func(s network.Stream) {
		slog := log.
			With(streamFields(s)).
			With(g)
		defer s.Close()

		ctx, cancel := context.WithTimeout(context.TODO(), timeout)
		defer cancel()

		log.Debug("handler started")
		defer func() { log.Debug("handler finished") }()

		if err := g.pushpull(ctx, s); err != nil {
			slog.WithError(err).Debug("peer exchange failed")
		}
	}
}

func (g *gossiper) pushpull(ctx context.Context, s network.Stream) error {
	var (
		j      syncutil.Join
		t, _   = ctx.Deadline()
		remote View
	)

	if err := s.SetDeadline(t); err != nil {
		return err
	}

	recs, err := g.LoadRecords()
	if err != nil {
		return err
	}

	recs = recs.Bind(sorted())
	oldest := recs.Bind(g.param.tail())

	local := append(
		recs.
			Bind(head(len(recs)-len(oldest))).
			Bind(shuffled()),
		oldest...)

	// push
	j.Go(func() error {
		defer s.CloseWrite()

		buffer := local.
			Bind(isNot(s.Conn().RemotePeer())).
			Bind(g.param.head()).
			Bind(appendLocal(g))

		enc := capnp.NewPackedEncoder(s)
		for _, g := range buffer {
			if err = enc.Encode(g.Message()); err != nil {
				break
			}
		}

		return err
	})

	// pull
	j.Go(func() error {
		defer s.CloseRead()

		r := io.LimitReader(s, g.param.mtu())

		dec := capnp.NewPackedDecoder(r)
		dec.MaxMessageSize = mtu

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
		err = g.MergeAndStore(local, remote)
	}

	return err
}

func (g *gossiper) MergeAndStore(local, remote View) error {
	if err := remote.Validate(); err != nil {
		return err
	}

	// Remove duplicates and combine local and remote records
	newLocal := local.
		Bind(merged(remote)).
		Bind(isNot(g.Record().PeerID))

	// Apply swapping
	s := min(g.param.S, max(len(newLocal)-g.param.C, 0))
	newLocal = newLocal.
		Bind(tail(len(newLocal) - s)).
		Bind(sorted())

	// Apply retention
	r := min(min(g.param.P, g.param.C), len(newLocal))
	maxDecay := min(r, max(len(newLocal)-g.param.C, 0))
	oldest := newLocal.Bind(tail(r)).Bind(decay(g.param.D, maxDecay))

	//Apply random eviction
	c := g.param.C - len(oldest)
	newLocal = newLocal.
		Bind(head(max(len(newLocal)-r, 0))).
		Bind(shuffled()).
		Bind(head(c))

	// Merge with oldest nodes
	newLocal = newLocal.
		Bind(merged(oldest))

	newLocal.incrHops()

	return g.StoreRecords(local, newLocal)
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
