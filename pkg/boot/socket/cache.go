package socket

import (
	"sync/atomic"

	"capnproto.org/go/capnp/v3"
	lru "github.com/hashicorp/golang-lru"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/record"
	"github.com/wetware/casm/internal/api/boot"
)

type RecordCache struct {
	rec   atomic.Value
	cache *lru.TwoQueueCache
}

func NewRecordCache(size int) *RecordCache {
	c, err := lru.New2Q(size)
	if err != nil {
		panic(err)
	}

	return &RecordCache{cache: c}
}

// Returns true if the cache was initialized with the host's record.
func (c *RecordCache) Initialized() bool {
	return c.rec.Load() == nil
}

func (c *RecordCache) Reset(e *record.Envelope) error {
	var rec peer.PeerRecord
	defer c.cache.Purge()
	defer c.rec.Store(&rec)

	return e.TypedRecord(&rec)
}

func (c *RecordCache) LoadRequest(pk crypto.PrivKey, ns string) (*record.Envelope, error) {
	if v, ok := c.cache.Get(keyRequest(ns)); ok {
		return v.(*record.Envelope), nil
	}

	return c.bind(pk, ns, bindRequest(pk))
}

func (c *RecordCache) LoadGradualRequest(pk crypto.PrivKey, ns string, dist uint8) (*record.Envelope, error) {
	if v, ok := c.cache.Get(keyGradual(ns, dist)); ok {
		return v.(*record.Envelope), nil
	}

	return c.bind(pk, ns, bindGradualRequest(pk, dist))
}

func (c *RecordCache) LoadResponse(pk crypto.PrivKey, ns string) (*record.Envelope, error) {
	if v, ok := c.cache.Get(keyResponse(ns)); ok {
		return v.(*record.Envelope), nil
	}

	return c.bind(pk, ns, bindResponse)
}

func (c *RecordCache) bind(pk crypto.PrivKey, ns string, bind func(*peer.PeerRecord) func(boot.Packet) error) (*record.Envelope, error) {
	pr := c.rec.Load().(*peer.PeerRecord)
	rec, err := newCacheEntry(ns, bind(pr))
	if err != nil {
		return nil, err
	}

	e, err := record.Seal(&rec, pk)
	if err == nil {
		c.cache.Add(ns, e)
	}

	return e, err
}

type (
	keyRequest        string
	keyResponse       string
	keyGradualRequest struct {
		ns   string
		dist uint8
	}
)

func keyGradual(ns string, dist uint8) keyGradualRequest {
	return keyGradualRequest{ns: ns, dist: dist}
}

func newCacheEntry(ns string, bind func(boot.Packet) error) (Record, error) {
	p, _ := newPacket(capnp.SingleSegment(nil), ns)
	return Record(p), bind(p)
}

func bindRequest(pk crypto.PrivKey) func(*peer.PeerRecord) func(boot.Packet) error {
	return func(_ *peer.PeerRecord) func(boot.Packet) error {
		return func(p boot.Packet) error {
			return bindID(p.SetRequest, pk)
		}
	}
}

func bindGradualRequest(pk crypto.PrivKey, dist uint8) func(*peer.PeerRecord) func(boot.Packet) error {
	return func(_ *peer.PeerRecord) func(boot.Packet) error {
		return func(p boot.Packet) error {
			p.SetGradualRequest()
			p.GradualRequest().SetDistance(dist)
			return bindID(p.GradualRequest().SetFrom, pk)
		}
	}
}

func bindID(bind func(s string) error, pk crypto.PrivKey) error {
	id, err := peer.IDFromPrivateKey(pk)
	if err == nil {
		err = bind(string(id))
	}
	return err
}

func bindResponse(r *peer.PeerRecord) func(boot.Packet) error {
	return func(p boot.Packet) error {
		p.SetResponse()

		if err := p.Response().SetPeer(string(r.PeerID)); err != nil {
			return err
		}

		addrs, err := p.Response().NewAddrs(int32(len(r.Addrs)))
		if err == nil {
			for i, addr := range r.Addrs {
				if err = addrs.Set(i, addr.Bytes()); err != nil {
					break
				}
			}
		}

		return err
	}
}

func newPacket(a capnp.Arena, ns string) (boot.Packet, error) {
	_, s, err := capnp.NewMessage(a)
	if err != nil {
		return boot.Packet{}, err
	}

	p, err := boot.NewRootPacket(s)
	if err != nil {
		return boot.Packet{}, err
	}

	return p, p.SetNamespace(ns)
}
