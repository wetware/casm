package pex

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/ioutil"
	"math/rand"
	"path"
	"time"

	"github.com/jbenet/goprocess"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/helpers"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/libp2p/go-libp2p-core/record"

	"github.com/lthibault/jitterbug/v2"
	"github.com/lthibault/log"
	ctxutil "github.com/lthibault/util/ctx"
	syncutil "github.com/lthibault/util/sync"

	protoutil "github.com/wetware/casm/pkg/util/proto"
)

const (
	Version               = "0.0.0"
	baseProto protocol.ID = "/casm/pex"
	Proto     protocol.ID = baseProto + "/" + Version
)

var initializers = [...]initFunc{
	checkHostIsListening,
	initAtomics,
	initSubscriptions,
	initStreamHandlers,
	initProcs,
	waitReady,
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
	ns   string
	log  logger
	tick time.Duration

	h      host.Host
	proc   goprocess.Process
	events event.Subscription
	atomic atomicValues

	maxSize     int
	newSelector ViewSelectorFactory
}

// New peer exchange.
func New(h host.Host, ns string, opt ...Option) (*PeerExchange, error) {
	var px = &PeerExchange{
		ns: ns,
		h:  h,
	}

	// set options
	for _, option := range withDefaults(opt) {
		option(px)
	}

	// initialize peer exchange
	var maybe breaker
	for _, fn := range initializers {
		maybe.Do(func() { maybe.Err = fn(px) })
	}

	return px, maybe.Err
}

func (px *PeerExchange) String() string             { return px.ns }
func (px *PeerExchange) Process() goprocess.Process { return px.proc }
func (px *PeerExchange) Close() error               { return px.proc.Close() }

// View returns a passively-updated view of the cluster.
func (px *PeerExchange) View() View {
	immut := px.atomic.view.Load() // DO NOT mutate
	v := make(View, len(immut))
	copy(v, immut)
	return v
}

// Join the namespace using a bootstrap peer.
//
// Join blocks until the underlying host is listening on at least one network
// address.
func (px *PeerExchange) Join(ctx context.Context, boot peer.AddrInfo) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		s     network.Stream
		maybe breaker
	)

	for _, fn := range []func(){
		func() { maybe.Err = px.h.Connect(ctx, boot) },                     // connect to boot peer
		func() { s, maybe.Err = px.h.NewStream(ctx, boot.ID, px.proto()) }, // open pex stream
		func() { maybe.Err = px.pushpull(ctx, s) },                         // perform initial gossip round
	} {
		maybe.Do(fn)
	}

	return maybe.Err
}

func (px *PeerExchange) gossip(ctx context.Context) (err error) {
	view := px.atomic.view.Load() // do not mutate!
	peers := view.IDs()
	rand.Shuffle(len(peers), peers.Swap)

	for _, id := range peers {
		switch err = px.gossipOne(ctx, id); err.(type) {
		case nil:
			return
		case streamError:
			px.log.With(err.(log.Loggable)).Debug("unable to connect")
		default:
			return
		}
	}

	// we get here either if len(peers) == 0, or if all peers are unreachable.
	return errors.New("orphaned host")
}

func (px *PeerExchange) gossipOne(ctx context.Context, id peer.ID) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second*15)
	defer cancel()

	s, err := px.h.NewStream(ctx, id, px.proto())
	if err != nil {
		return streamError{Peer: id, error: err}
	}

	return px.pushpull(ctx, s)
}

func (px *PeerExchange) proto() protocol.ID {
	return protoutil.AppendStrings(Proto, px.ns)
}

func (px *PeerExchange) pushpull(ctx context.Context, s network.Stream) error {
	// NOTE:  v MUST NOT be mutated!
	defer s.Close()

	var (
		t, _ = ctx.Deadline()
		j    syncutil.Join
	)

	if err := s.SetDeadline(t); err != nil {
		return err
	}

	// push
	j.Go(func() error {
		defer s.CloseWrite()

		v := px.atomic.view.Load()

		// copy view and append local peer's gossip record
		rec := make(View, len(v), len(v)+1)
		copy(rec, v)
		rec = append(rec, px.atomic.record.Load())

		// marshal & sign 'rec'
		env, err := record.Seal(&rec, px.privkey())
		if err != nil {
			return err
		}

		b, err := env.Marshal()
		if err != nil {
			return err
		}

		_, err = io.Copy(s, bytes.NewReader(b))
		return err
	})

	// pull
	j.Go(func() error {
		defer s.CloseRead()

		var remote View

		// defensively limit buffer size; assume 1kb per record
		b, err := ioutil.ReadAll(io.LimitReader(s, int64(px.maxSize)*1024))
		if err != nil {
			return err
		}

		env, err := record.ConsumeTypedEnvelope(b, &remote)
		if err != nil {
			return err
		}

		if err = remote.Validate(env); err != nil {
			//  TODO(security):  implement peer scoring system and punish peers
			//					 whose messages fail validation.
			return err
		}

		remote.incrHops()
		return px.mergeAndSelect(remote)
	})

	return j.Wait()
}

func (px *PeerExchange) mergeAndSelect(remote View) error {
	px.atomic.Lock()
	defer px.atomic.Unlock()
	/*
	 *  CAUTION:  'local' MUST NOT be mutated!
	 */

	local := px.atomic.view.Load()
	sender := remote.last()
	selectv := px.newSelector(px.h, &sender, px.maxSize)

	return px.atomic.view.Store(selectv(px.merge(local, remote)))
}

func (px *PeerExchange) merge(local, remote View) View {
	remote = append(remote, local...) // merge local view into remote.

	// Remove duplicate records.
	merged := remote[:0]
	for _, g := range remote {
		// skip record if it came from us
		if g.PeerID == px.h.ID() {
			continue
		}

		have, found := merged.find(g)

		/* Select if:

		unique  ...   more recent   ...  less diffused  */
		if !found || g.Seq > have.Seq || g.Hop < have.Hop {
			merged = append(merged, g)
		}
	}

	return merged
}

func (px *PeerExchange) updateLocalRecord(ev event.EvtLocalAddressesUpdated) error {
	g, err := NewGossipRecordFromEvent(ev)
	if err == nil {
		px.atomic.record.Store(g)
	}
	return err
}

func (px *PeerExchange) privkey() crypto.PrivKey {
	return px.h.Peerstore().PrivKey(px.h.ID())
}

/*
 * Set-up functions
 */

type initFunc func(*PeerExchange) error

func checkHostIsListening(px *PeerExchange) (err error) {
	if len(px.h.Addrs()) == 0 {
		err = errors.New("host not accepting connections")
	}
	return
}

func initAtomics(px *PeerExchange) (err error) {
	px.atomic, err = newAtomicValues(px.h.EventBus())
	return
}

func initSubscriptions(px *PeerExchange) (err error) {
	px.events, err = px.h.EventBus().Subscribe([]interface{}{
		new(event.EvtLocalAddressesUpdated),
		new(EvtViewUpdated),
		new(EvtLocalRecordUpdated),
	})

	return
}

func initStreamHandlers(px *PeerExchange) error {
	versionOK, err := helpers.MultistreamSemverMatcher(Proto)
	if err == nil {
		px.h.SetStreamHandlerMatch(
			Proto,
			func(s string) bool { return versionOK(path.Dir(s)) && px.ns == path.Base(s) },
			func(s network.Stream) {
				defer s.Close()

				const d = time.Second * 15
				ctx, cancel := context.WithTimeout(ctxutil.FromChan(px.proc.Closing()), d)
				defer cancel()

				if err := px.pushpull(ctx, s); err != nil {
					px.log.WithStream(s).WithError(err).
						Debug("error handling gosisp")
				}
			})
	}

	return err
}

func initProcs(px *PeerExchange) error {
	px.proc = px.h.Network().Process().Go(func(proc goprocess.Process) {
		defer px.h.RemoveStreamHandler(Proto)

		<-proc.Closing()
	})
	px.proc.SetTeardown(px.atomic.CloseAll)

	px.proc.Go(func(proc goprocess.Process) {
		ctx := asClosingContext(proc)

		ticker := jitterbug.New(px.tick, jitterbug.Uniform{
			Min:    px.tick / 2,
			Source: rand.New(rand.NewSource(time.Now().UnixNano())),
		})
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := px.gossip(ctx); err != nil {
					px.log.WithError(err).Debug("gossip round failed")
				}

			case <-ctx.Done():
				return
			}
		}

	})

	px.proc.Go(func(proc goprocess.Process) {
		for {
			select {
			case v := <-px.events.Out():
				switch ev := v.(type) {
				case event.EvtLocalAddressesUpdated:
					if err := px.updateLocalRecord(ev); err != nil {
						px.log.WithError(err).Error("invalid peer record in event")
					}

				case EvtViewUpdated:
					px.log.With(ev).Trace("view updated")

				case EvtLocalRecordUpdated:
					px.log.With(ev).Trace("local record updated")

				}

			case <-proc.Closing():
				return
			}
		}
	}).SetTeardown(px.events.Close)

	return nil
}

func waitReady(px *PeerExchange) error {
	sub, err := px.h.EventBus().Subscribe(new(EvtLocalRecordUpdated))
	if err != nil {
		return err
	}
	defer sub.Close()

	// We know the event is coming, thanks to 'checkHostIsListening' and the
	// fact that 'event.EvtLocalAddressUpdated' is stateful.

	select {
	case <-px.proc.Closing():
		return errors.New("closing")

	case v, ok := <-sub.Out():
		if !ok || v == nil {
			panic("nil event")
		}

		return nil
	}
}

func asClosingContext(p goprocess.Process) context.Context {
	return ctxutil.FromChan(p.Closing())
}
