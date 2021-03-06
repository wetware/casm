package pulse_test

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/stretchr/testify/assert"
	mock_libp2p "github.com/wetware/casm/internal/mock/libp2p"
	mock_pulse "github.com/wetware/casm/internal/mock/pkg/cluster/pulse"
	"github.com/wetware/casm/pkg/cluster/pulse"
)

var t0 = time.Date(2020, 4, 9, 8, 0, 0, 0, time.UTC)

func TestValidator(t *testing.T) {
	t.Parallel()
	t.Helper()

	t.Run("Heartbeat", func(t *testing.T) {
		t.Parallel()
		t.Helper()

		t.Run("Reject_unmarshal_fails", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			rt := mock_pulse.NewMockRoutingTable(ctrl)
			e := mock_libp2p.NewMockEmitter(ctrl)
			validate := pulse.NewValidator(rt, e)

			res := validate(context.Background(), newPeerID(),
				&pubsub.Message{Message: &pb.Message{}})
			assert.Equal(t, pubsub.ValidationReject, res)
		})

		// t.Run("Ignore_stale_record", func(t *testing.T) {
		// 	a, err := newAnnouncement(capnp.SingleSegment(nil))
		// 	require.NoError(t, err)

		// 	hb, err := a.NewHeartbeat()
		// 	require.NoError(t, err)

		// 	hb.SetTTL(time.Hour)

		// 	b, err := a.MarshalBinary()
		// 	require.NoError(t, err)

		// 	id := newPeerID()
		// 	msg := &pubsub.Message{Message: &pb.Message{
		// 		From:  []byte(id),
		// 		Seqno: []byte{0, 0, 0, 0, 0, 0, 0, 8},
		// 		Data:  b,
		// 	}}

		// 	res := validate(context.Background(), id, msg)
		// 	require.Equal(t, pubsub.ValidationAccept, res)

		// 	msg.Seqno = []byte{0, 0, 0, 0, 0, 0, 0, 1}
		// 	res = validate(context.Background(), id, msg)
		// 	require.Equal(t, pubsub.ValidationIgnore, res)
		// })
	})

	// 	t.Run("JoinLeave", func(t *testing.T) {
	// 		t.Helper()

	// 		for _, tt := range []struct {
	// 			which cluster.Announcement_Which
	// 			id    peer.ID
	// 			want  pubsub.ValidationResult
	// 		}{
	// 			{
	// 				which: cluster.Announcement_Which_join,
	// 				id:    newPeerID(),
	// 				want:  pubsub.ValidationAccept,
	// 			},
	// 			{
	// 				which: cluster.Announcement_Which_leave,
	// 				id:    newPeerID(),
	// 				want:  pubsub.ValidationAccept,
	// 			},
	// 		} {
	// 			t.Run(tt.which.String(), func(t *testing.T) {
	// 				ctrl := gomock.NewController(t)
	// 				defer ctrl.Finish()

	// 				e := mock_libp2p.NewMockEmitter(ctrl)
	// 				e.EXPECT().
	// 					Emit(gomock.AssignableToTypeOf(EvtMembershipChanged{})).
	// 					Return(nil).
	// 					Times(1)

	// 				validate := m.NewValidator(e)

	// 				a, err := newAnnouncement(capnp.SingleSegment(nil))
	// 				require.NoError(t, err)

	// 				switch tt.which {
	// 				case cluster.Announcement_Which_join:
	// 					err = a.SetJoin(tt.id)
	// 				case cluster.Announcement_Which_leave:
	// 					err = a.SetLeave(tt.id)
	// 				}

	// 				require.NoError(t, err)

	// 				b, err := a.MarshalBinary()
	// 				require.NoError(t, err)

	// 				id := newPeerID()
	// 				msg := &pubsub.Message{Message: &pb.Message{
	// 					From:  []byte(id),
	// 					Seqno: []byte{0, 0, 0, 0, 0, 0, 0, 1},
	// 					Data:  b,
	// 				}}

	// 				got := validate(context.Background(), tt.id, msg)
	// 				require.Equal(t, tt.want, got)
	// 			})
	// 		}
	// 	})
}

func newPeerID() peer.ID {
	sk, _, err := crypto.GenerateECDSAKeyPair(rand.Reader)
	if err != nil {
		panic(err)
	}

	id, err := peer.IDFromPrivateKey(sk)
	if err != nil {
		panic(err)
	}

	return id
}
