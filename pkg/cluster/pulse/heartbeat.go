package pulse

import (
	"time"

	"capnproto.org/go/capnp/v3"
	"github.com/wetware/casm/internal/api/pulse"
)

type (
	Meta     = pulse.Heartbeat_meta
	MetaType = pulse.Heartbeat_meta_Which
)

var (
	MetaType_None   = pulse.Heartbeat_meta_Which_none
	MetaType_Text   = pulse.Heartbeat_meta_Which_text
	MetaType_Binary = pulse.Heartbeat_meta_Which_binary
	MetaType_Ptr    = pulse.Heartbeat_meta_Which_pointer
)

type Heartbeat struct{ h pulse.Heartbeat }

func NewHeartbeat(a capnp.Arena) (Heartbeat, error) {
	_, seg, err := capnp.NewMessage(a)
	if err != nil {
		return Heartbeat{}, err
	}

	h, err := pulse.NewHeartbeat(seg)
	return Heartbeat{h}, err
}

func (hb Heartbeat) SetTTL(d time.Duration) { hb.h.SetTtl(int64(d)) }
func (hb Heartbeat) TTL() time.Duration     { return time.Duration(hb.h.Ttl()) }
func (hb Heartbeat) Meta() Meta             { return hb.h.Meta() }

func (hb Heartbeat) MarshalBinary() ([]byte, error) {
	return hb.h.Message().MarshalPacked()
}

func (hb *Heartbeat) UnmarshalBinary(b []byte) error {
	msg, err := capnp.UnmarshalPacked(b)
	if err == nil {
		hb.h, err = pulse.ReadRootHeartbeat(msg)
	}

	return err
}
