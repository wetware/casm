package cluster

import (
	"context"
	"testing"
	"time"

	"capnproto.org/go/capnp/v3"
	"github.com/golang/mock/gomock"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wetware/casm/internal/api/cluster"
	mock_libp2p "github.com/wetware/casm/internal/mock/libp2p"
)

var t0 = time.Date(2020, 4, 9, 8, 0, 0, 0, time.UTC)

func TestValidator(t *testing.T) {
	t.Parallel()
	t.Helper()

	var m routingTable
	m.Store(state{})

	t.Run("Heartbeat", func(t *testing.T) {
		t.Helper()

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		e := mock_libp2p.NewMockEmitter(ctrl)
		validate := m.NewValidator(e)

		t.Run("Reject_unmarshal_fails", func(t *testing.T) {
			res := validate(context.Background(), newPeerID(),
				&pubsub.Message{Message: &pb.Message{}})
			assert.Equal(t, pubsub.ValidationReject, res)
		})

		t.Run("Ignore_stale_record", func(t *testing.T) {
			a, err := newAnnouncement(capnp.SingleSegment(nil))
			require.NoError(t, err)

			hb, err := a.NewHeartbeat()
			require.NoError(t, err)

			hb.SetTTL(time.Hour)

			b, err := a.MarshalBinary()
			require.NoError(t, err)

			id := newPeerID()
			msg := &pubsub.Message{Message: &pb.Message{
				From:  []byte(id),
				Seqno: []byte{0, 0, 0, 0, 0, 0, 0, 8},
				Data:  b,
			}}

			res := validate(context.Background(), id, msg)
			require.Equal(t, pubsub.ValidationAccept, res)

			msg.Seqno = []byte{0, 0, 0, 0, 0, 0, 0, 1}
			res = validate(context.Background(), id, msg)
			require.Equal(t, pubsub.ValidationIgnore, res)
		})
	})

	t.Run("JoinLeave", func(t *testing.T) {
		t.Helper()

		for _, tt := range []struct {
			which cluster.Announcement_Which
			id    peer.ID
			want  pubsub.ValidationResult
		}{
			{
				which: cluster.Announcement_Which_join,
				id:    newPeerID(),
				want:  pubsub.ValidationAccept,
			},
			{
				which: cluster.Announcement_Which_leave,
				id:    newPeerID(),
				want:  pubsub.ValidationAccept,
			},
		} {
			t.Run(tt.which.String(), func(t *testing.T) {
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()

				e := mock_libp2p.NewMockEmitter(ctrl)
				e.EXPECT().
					Emit(gomock.AssignableToTypeOf(EvtMembershipChanged{})).
					Return(nil).
					Times(1)

				validate := m.NewValidator(e)

				a, err := newAnnouncement(capnp.SingleSegment(nil))
				require.NoError(t, err)

				switch tt.which {
				case cluster.Announcement_Which_join:
					err = a.SetJoin(tt.id)
				case cluster.Announcement_Which_leave:
					err = a.SetLeave(tt.id)
				}

				require.NoError(t, err)

				b, err := a.MarshalBinary()
				require.NoError(t, err)

				id := newPeerID()
				msg := &pubsub.Message{Message: &pb.Message{
					From:  []byte(id),
					Seqno: []byte{0, 0, 0, 0, 0, 0, 0, 1},
					Data:  b,
				}}

				got := validate(context.Background(), tt.id, msg)
				require.Equal(t, tt.want, got)
			})
		}
	})
}

func TestRoutingTable(t *testing.T) {
	t.Parallel()

	var (
		m  routingTable
		id = newPeerID()
	)

	a, err := newAnnouncement(capnp.SingleSegment(nil))
	require.NoError(t, err)

	hb, err := a.NewHeartbeat()
	require.NoError(t, err)
	hb.SetTTL(time.Second)

	m.Store(state{})

	assert.NotPanics(t, func() { m.Advance(t0) },
		"advancing an empty filter should not panic.")

	assert.False(t, contains(&m, id),
		"canary failed:  ID should not be present in empty filter.")

	assert.True(t, m.Upsert(id, peerRecord{0, hb}),
		"upsert of new ID should succeed.")

	assert.True(t, contains(&m, id),
		"filter should contain ID %s after INSERT.", id)

	assert.True(t, m.Upsert(id, peerRecord{3, hb}),
		"upsert of existing ID with higher sequence number should succeed.")

	assert.True(t, contains(&m, id),
		"filter should contain ID %s after UPDATE", id)

	assert.False(t, m.Upsert(id, peerRecord{1, hb}),
		"upsert of existing ID with lower sequence number should fail.")

	assert.True(t, contains(&m, id),
		"filter should contain ID %s after FAILED UPDATE.", id)

	assert.Contains(t, peers(&m), id,
		"ID should appear in peer.IDSlice")

	m.Advance(t0.Add(time.Millisecond * 100))
	assert.True(t, contains(&m, id),
		"advancing by less than the TTL amount should NOT cause eviction.")

	m.Advance(t0.Add(time.Second))
	assert.False(t, contains(&m, id),
		"advancing by more than the TTL amount should cause eviction")
}

func contains(m *routingTable, id peer.ID) bool {
	_, ok := handle.Get(m.Load().n, id)
	return ok
}

func peers(m *routingTable) (ps peer.IDSlice) {
	for it := handle.Iter(m.Load().n); it.Next(); {
		ps = append(ps, it.Key.(peer.ID))
	}
	return
}
