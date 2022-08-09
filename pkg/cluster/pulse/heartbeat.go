package pulse

import (
	"math/rand"
	"time"

	"capnproto.org/go/capnp/v3"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/wetware/casm/internal/api/pulse"
)

type Heartbeat pulse.Heartbeat

func NewHeartbeat() Heartbeat {
	_, seg := capnp.NewSingleSegmentMessage(nil)
	h, err := pulse.NewRootHeartbeat(seg)
	if err != nil {
		panic(err)
	}

	h.SetId(rand.Uint32())

	return Heartbeat(h)
}

func (h Heartbeat) Loggable() map[string]any {
	return map[string]any{
		"ttl":      h.TTL(),
		"instance": h.ID(),
	}
}

func (h Heartbeat) TTL() time.Duration {
	ms := pulse.Heartbeat(h).Ttl()
	return time.Millisecond * time.Duration(ms)
}

func (h Heartbeat) ID() uint32 {
	return pulse.Heartbeat(h).Id()
}

func (h Heartbeat) Hostname() (string, error) {
	return pulse.Heartbeat(h).Hostname()
}

func (h Heartbeat) Meta() (Meta, error) {
	meta, err := pulse.Heartbeat(h).Meta()
	return Meta(meta), err
}

func (h Heartbeat) Message() *capnp.Message {
	return pulse.Heartbeat(h).Message()
}

func (h *Heartbeat) ReadMessage(m *capnp.Message) (err error) {
	*(*pulse.Heartbeat)(h), err = pulse.ReadRootHeartbeat(m)
	return
}

func (h *Heartbeat) Bind(msg *pubsub.Message) (err error) {
	var m *capnp.Message
	if m, err = capnp.UnmarshalPacked(msg.Data); err == nil {
		msg.ValidatorData = h
		err = h.ReadMessage(m)
	}

	return
}

func (h Heartbeat) MarshalBinary() ([]byte, error) {
	return pulse.Heartbeat(h).Message().MarshalPacked()
}

func (h *Heartbeat) UnmarshalBinary(b []byte) error {
	msg, err := capnp.UnmarshalPacked(b)
	if err == nil {
		*(*pulse.Heartbeat)(h), err = pulse.ReadRootHeartbeat(msg)
	}

	return err
}

type Meta pulse.Heartbeat_Field_List

func (m Meta) Len() int {
	return pulse.Heartbeat_Field_List(m).Len()
}

func (m Meta) Get(key string) (string, error) {
	for i := 0; i < m.Len(); i++ {
		field := pulse.Heartbeat_Field_List(m).At(i)

		k, err := field.Key()
		if err != nil {
			return "", err
		}

		if k == key {
			return field.Value()
		}
	}

	return "", nil
}
