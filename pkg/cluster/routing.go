package cluster

import (
	"context"
	"encoding/binary"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/lthibault/treap"
	"github.com/wetware/casm/internal/api/cluster"
)

/*
 * model.go specifies a cluster model with PA/EL guarantees.
 */

var handle = treap.Handle{
	CompareKeys:    unsafePeerIDComparator,
	CompareWeights: unsafeTimeComparator,
}

type peerRecord struct {
	Seq       uint64
	Heartbeat heartbeat
}

type state struct {
	t time.Time
	n *treap.Node
}

type routingTable atomic.Value

func (m *routingTable) NewValidator(e event.Emitter) pubsub.ValidatorEx {
	return func(_ context.Context, id peer.ID, msg *pubsub.Message) pubsub.ValidationResult {
		var a announcement
		if err := a.UnmarshalBinary(msg.Data); err != nil {
			return pubsub.ValidationReject
		}

		msg.ValidatorData = a

		switch a.Which() {
		case cluster.Announcement_Which_heartbeat:
			if hb, err := a.Heartbeat(); err == nil {
				if m.Upsert(id, record(msg, hb)) {
					return pubsub.ValidationAccept
				}

				// heartbeat is valid, but we have a more recent one.
				return pubsub.ValidationIgnore
			}

		case cluster.Announcement_Which_join, cluster.Announcement_Which_leave:
			if validateJoinLeaveID(a) {
				_ = e.Emit(newPeerEvent(id, msg, a))
				return pubsub.ValidationAccept
			}
		}

		// assume the worst...
		return pubsub.ValidationReject
	}
}

func (m *routingTable) Advance(t time.Time) {
	var old, new state
	for /* CAS loop */ {

		// is there any data?
		if old = m.Load(); old.n != nil {
			// evict stale entries
			for new = old; expired(t, new); {
				new = merge(t, new)
			}
		} else {
			// just update the time
			new = state{t: t, n: old.n}
		}

		if m.CompareAndSwap(old, new) {
			break
		}
	}
}

func (m *routingTable) Upsert(id peer.ID, r peerRecord) bool {
	var ok, created bool

	for {
		old := m.Load()
		new := old

		// upsert if seq is greater than the value stored in the treap -- non-blocking.
		new.n, created = handle.UpsertIf(new.n, id, r, old.t.Add(r.Heartbeat.TTL()), func(n *treap.Node) bool {
			ok = newer(n, r) // set return value for outer closure
			return ok
		})

		if m.CompareAndSwap(old, new) { // atomic
			break
		}
	}

	// The message should be processed iff the incoming message's sequence number is
	// greater than the one in the treap (ok==true) OR the id was just inserted into the
	// treap (created==true).
	return ok || created
}

func (m *routingTable) Store(s state) { (*atomic.Value)(m).Store(s) }

func (m *routingTable) Load() state {
	v := (*atomic.Value)(m).Load()
	return *(*state)((*ifaceWords)(unsafe.Pointer(&v)).data)
}

func (m *routingTable) CompareAndSwap(old, new state) bool {
	return (*atomic.Value)(m).CompareAndSwap(old, new)
}

func merge(t time.Time, s state) state {
	return state{
		t: t,
		n: handle.Merge(s.n.Left, s.n.Right),
	}
}

func expired(t time.Time, s state) (ok bool) {
	if s.n != nil {
		ok = handle.CompareWeights(s.n.Weight, t) <= 0
	}

	return
}

func record(msg *pubsub.Message, hb heartbeat) peerRecord {
	return peerRecord{
		Seq:       seqno(msg),
		Heartbeat: hb,
	}
}

func newer(n *treap.Node, r peerRecord) bool {
	return recordCastUnsafe(n.Value).Seq < r.Seq
}

func seqno(msg *pubsub.Message) uint64 {
	return binary.BigEndian.Uint64(msg.GetSeqno())
}

func validateJoinLeaveID(a announcement) bool {
	var (
		s   string
		err error
	)

	switch a.Which() {
	case cluster.Announcement_Which_join:
		s, err = a.Join()
	case cluster.Announcement_Which_leave:
		s, err = a.Leave()
	}

	if err == nil {
		_, err = peer.IDFromString(s)
	}

	return err == nil
}
