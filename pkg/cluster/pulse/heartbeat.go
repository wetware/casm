package pulse

import (
	"math/rand"
	"time"

	"capnproto.org/go/capnp/v3"
	pubsub "github.com/libp2p/go-libp2p-pubsub"

	api "github.com/wetware/casm/internal/api/routing"
	"github.com/wetware/casm/pkg/cluster/routing"
)

const DefaultTTL = time.Second * 10

type Heartbeat struct{ api.Heartbeat }

func NewHeartbeat() Heartbeat {
	_, seg := capnp.NewSingleSegmentMessage(nil)
	h, _ := api.NewRootHeartbeat(seg) // single segment never fails
	h.SetInstance(rand.Uint32())

	return Heartbeat{h}
}

func (h Heartbeat) Loggable() map[string]any {
	return map[string]any{
		"ttl":      h.TTL(),
		"instance": h.Instance(),
	}
}

func (h Heartbeat) SetTTL(d time.Duration) {
	ms := d / time.Millisecond
	h.SetTtl(uint32(ms))
}

func (h Heartbeat) TTL() (d time.Duration) {
	if d = time.Millisecond * time.Duration(h.Ttl()); d == 0 {
		d = DefaultTTL
	}

	return
}

func (h Heartbeat) SetInstance(id routing.ID) {
	h.Heartbeat.SetInstance(uint32(id))
}

func (h Heartbeat) Instance() routing.ID {
	return routing.ID(h.Heartbeat.Instance())
}

func (h Heartbeat) Meta() (routing.Meta, error) {
	meta, err := h.Heartbeat.Meta()
	return routing.Meta(meta), err
}

func (h Heartbeat) SetMeta(fields []routing.Field) error {
	meta, err := h.NewMeta(int32(len(fields)))
	if err != nil {
		return err
	}

	for i, f := range fields {
		if err = meta.Set(i, f.String()); err != nil {
			break
		}
	}

	return err
}

func (h *Heartbeat) ReadMessage(m *capnp.Message) (err error) {
	h.Heartbeat, err = api.ReadRootHeartbeat(m)
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
