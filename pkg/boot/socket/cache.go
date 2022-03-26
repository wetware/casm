package socket

import (
	"sync/atomic"

	"capnproto.org/go/capnp/v3"
	lru "github.com/hashicorp/golang-lru"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/record"
	"github.com/wetware/casm/internal/api/boot"
)

// Sealer is a higher-order function capable of sealing a
// record for a specific peer. It prevents the cache from
// having to manage private keys.
type Sealer func(record.Record) (*record.Envelope, error)

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

// Reset the cache by passing an envelope containing a host's
// signed peer.PeerRecord.  This invalidates previous entries
// and ensures all future records reference e's addresses.
func (c *RecordCache) Reset(e *record.Envelope) error {
	var rec peer.PeerRecord
	defer c.cache.Purge()
	defer c.rec.Store(&rec)

	return e.TypedRecord(&rec)
}

// LoadRequest searches the cache for a signed request packet for ns
// and returns it, if found. Else, it creates and signs a new packet
// and adds it to the cache.
func (c *RecordCache) LoadRequest(seal Sealer, id peer.ID, ns string) (*record.Envelope, error) {
	if v, ok := c.cache.Get(keyRequest(ns)); ok {
		return v.(*record.Envelope), nil
	}

	return c.bind(request(id), seal, ns)
}

// LoadSurveyRequest searches the cache for a signed survey packet
// with distance 'dist', and returns it if found. Else, it creates
// and signs a new survey-request packet and adds it to the cache.
func (c *RecordCache) LoadSurveyRequest(seal Sealer, id peer.ID, ns string, dist uint8) (*record.Envelope, error) {
	if v, ok := c.cache.Get(keyGradual(ns, dist)); ok {
		return v.(*record.Envelope), nil
	}

	return c.bind(surveyRequest(id, dist), seal, ns)
}

// LoadResponse searches the cache for a signed response packet for ns
// and returns it, if found. Else, it creates and signs a new response
// packet and adds it to the cache.
func (c *RecordCache) LoadResponse(seal Sealer, ns string) (*record.Envelope, error) {
	if v, ok := c.cache.Get(keyResponse(ns)); ok {
		return v.(*record.Envelope), nil
	}

	return c.bind(response(c.rec.Load().(*peer.PeerRecord)), seal, ns)
}

type bindFunc func(boot.Packet) error

func (c *RecordCache) bind(bind bindFunc, seal Sealer, ns string) (e *record.Envelope, err error) {
	if e, err = newCacheEntry(bind, seal, ns); err == nil {
		c.cache.Add(ns, e)
	}

	return
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

func newCacheEntry(bind bindFunc, seal Sealer, ns string) (*record.Envelope, error) {
	p, err := newPacket(capnp.SingleSegment(nil), ns)
	if err != nil {
		return nil, err
	}

	if err = bind(p); err != nil {
		return nil, err
	}

	return seal((*Record)(&p))
}

func request(from peer.ID) func(boot.Packet) error {
	return func(p boot.Packet) error {
		return p.SetRequest(string(from))
	}
}

func surveyRequest(from peer.ID, dist uint8) bindFunc {
	return func(p boot.Packet) error {
		p.SetGradualRequest()
		p.GradualRequest().SetDistance(dist)
		return p.GradualRequest().SetFrom(string(from))
	}
}

func response(r *peer.PeerRecord) bindFunc {
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
