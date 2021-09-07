package cluster

import (
	"time"

	"capnproto.org/go/capnp/v3"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/wetware/casm/internal/api/cluster"
)

type Hook func(Heartbeat)

type Heartbeat interface {
	TTL() time.Duration
	SetTTL(time.Duration)
	Record() Record
}

type (
	Record     = cluster.Announcement_Heartbeat_record
	RecordType = cluster.Announcement_Heartbeat_record_Which
)

var (
	RecordType_None   = cluster.Announcement_Heartbeat_record_Which_none
	RecordType_Text   = cluster.Announcement_Heartbeat_record_Which_text
	RecordType_Binary = cluster.Announcement_Heartbeat_record_Which_binary
	RecordType_Ptr    = cluster.Announcement_Heartbeat_record_Which_pointer
)

type announcement struct{ cluster.Announcement }

func newAnnouncement(arena capnp.Arena) (announcement, error) {
	var (
		a         announcement
		_, s, err = capnp.NewMessage(arena)
	)

	if err == nil {
		a.Announcement, err = cluster.NewRootAnnouncement(s)
	}

	return a, err
}

func (a announcement) NewHeartbeat() (heartbeat, error) {
	hb, err := a.Announcement.NewHeartbeat()
	return heartbeat(hb), err
}

func (a announcement) Heartbeat() (heartbeat, error) {
	hb, err := a.Announcement.Heartbeat()
	return heartbeat(hb), err
}

func (a announcement) SetJoin(id peer.ID) error {
	return a.Announcement.SetJoin(string(id))
}

func (a announcement) SetLeave(id peer.ID) error {
	return a.Announcement.SetLeave(string(id))
}

func (a announcement) MarshalBinary() ([]byte, error) {
	return a.Message().MarshalPacked()
}

func (a *announcement) UnmarshalBinary(b []byte) error {
	msg, err := capnp.UnmarshalPacked(b)
	if err == nil {
		a.Announcement, err = cluster.ReadRootAnnouncement(msg)
	}

	return err
}

type heartbeat cluster.Announcement_Heartbeat

func (hb heartbeat) SetTTL(d time.Duration) {
	(cluster.Announcement_Heartbeat)(hb).SetTtl(int64(d))
}

func (hb heartbeat) TTL() time.Duration {
	return time.Duration((cluster.Announcement_Heartbeat)(hb).Ttl())
}

func (hb heartbeat) Record() Record {
	return cluster.Announcement_Heartbeat(hb).Record()
}
