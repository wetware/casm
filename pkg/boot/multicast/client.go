package multicast

import (
	"context"

	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/record"
	"github.com/pkg/errors"
	"github.com/wetware/casm/internal/api/boot"
)

type ResponseStream interface {
	Responses() <-chan boot.MulticastPacket
}

// Client emits QUERY packets and awaits RESPONSE packet from peers.
type Client struct {
	cq   <-chan struct{}
	qs   chan<- outgoing
	add  chan addSink
	rm   chan func()
	recv ResponseStream
}

// NewMulticast client takes a UDP multiaddr that designates a
// multicast group and returns a multicast discovery client.
func NewClient(ctx context.Context, cfg Config) (Client, error) {
	return newClientFactory(ctx).
		Bind(clientConfig(cfg)).
		Bind(clientQueryLoop(cfg)).
		Bind(clientSendLoop(cfg)).
		Return()
}

func (c Client) FindPeers(ctx context.Context, ns string, opt ...discovery.Option) (<-chan peer.AddrInfo, error) {
	opts, err := c.options(opt)
	if err != nil {
		return nil, err
	}

	pkt, err := packetPool.GetQueryPacket(ns)
	if err != nil {
		return nil, err
	}
	defer packetPool.Put(pkt)

	out := make(chan peer.AddrInfo, 1)
	sink := &sink{
		NS:   ns,
		ch:   out,
		opts: opts,
	}

	cherr := chErrPool.Get()
	defer chErrPool.Put(cherr)

	add := addSink{
		C:   ctx,
		S:   sink,
		P:   pkt,
		Err: cherr,
	}

	select {
	case c.add <- add:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.cq:
		return nil, errors.New("closing")
	}

	select {
	case err = <-cherr:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.cq:
		return nil, errors.New("closing")
	}

	return out, err
}

func (c Client) options(opt []discovery.Option) (opts *discovery.Options, err error) {
	opts = &discovery.Options{}
	if err = opts.Apply(opt...); err == nil {
		if opts.Limit == 0 {
			opts.Limit = -1
		}
	}
	return
}

func (c Client) addSink(m clientTopicManager, add addSink) {
	var (
		err = errors.New("closing")
		ctx = add.C
	)

	m.AddSink(add.C, c.cq, add.S)

	cherr := chErrPool.Get()
	defer chErrPool.Put(cherr)

	select {
	case <-c.cq:
	case <-ctx.Done():
	case c.qs <- outgoing{
		C:   add.C,
		P:   add.P,
		Err: cherr,
	}:
	}

	select {
	case <-c.cq:
	case <-ctx.Done():
	case err = <-cherr:
	}

	if err != nil {
		add.S.Close()
	} else {
		go func() {
			select {
			case <-c.cq:
			case <-ctx.Done():
				select {
				case <-c.cq:
				case c.rm <- add.S.Close:
				}
			}
		}()
	}

	add.Err <- err
}

func (c Client) handleResponse(m clientTopicManager, p boot.MulticastPacket) error {
	var rec peer.PeerRecord

	res, err := p.Response()
	if err != nil {
		return err
	}

	ns, err := res.Ns()
	if err != nil {
		return err
	}

	// are we tracking this topic?
	if t, ok := m[ns]; ok {
		b, err := res.SignedEnvelope()
		if err != nil {
			return err
		}

		_, err = record.ConsumeTypedEnvelope(b, &rec)
		if err != nil {
			return err
		}

		t.Consume(c.cq, rec)
	}

	return nil
}

type clientFactory struct {
	ctx    context.Context
	cancel context.CancelFunc

	err error
	c   Client
}

func justClient(c Client) clientFactory { return clientFactory{c: c} }

// func clientFailure(err error) clientFactory { return clientFactory{err: err} }

func (cf clientFactory) Bind(f func(context.Context, Client) clientFactory) clientFactory {
	if cf.err != nil {
		cf.cancel()
		return cf
	}

	return f(cf.ctx, cf.c)
}

func (cf clientFactory) Return() (Client, error) { return cf.c, cf.err }

func newClientFactory(ctx context.Context) (f clientFactory) {
	f.ctx, f.cancel = context.WithCancel(ctx)
	return
}

func clientConfig(cfg Config) func(context.Context, Client) clientFactory {
	return func(ctx context.Context, c Client) clientFactory {
		c.cq = ctx.Done()
		c.qs = cfg.queryChan()
		c.add = make(chan addSink)
		c.rm = make(chan func())
		c.recv = cfg.router(ctx)
		return justClient(c)
	}
}

func clientQueryLoop(cfg Config) func(context.Context, Client) clientFactory {
	return func(ctx context.Context, c Client) clientFactory {
		var m = make(clientTopicManager)

		go func() {
			defer close(c.qs)
			defer m.Close()
			defer close(c.rm)
			defer close(c.add)

			for {
				select {
				case add := <-c.add:
					c.addSink(m, add)

				case free := <-c.rm:
					free()

				case r, ok := <-c.recv.Responses():
					if !ok {
						return
					}

					if err := c.handleResponse(m, r); err != nil {
						cfg.logger().WithError(err).Debug("malformed response")
						continue
					}
				}
			}
		}()

		return justClient(c)
	}
}

func clientSendLoop(cfg Config) func(context.Context, Client) clientFactory {
	return func(ctx context.Context, c Client) clientFactory {
		go sendLoop(cfg)
		return justClient(c)
	}
}

type clientTopicManager map[string]*clientTopic

func (m clientTopicManager) Close() {
	for _, t := range m {
		for _, s := range t.ss {
			s.Close()
		}
	}
}

func (m clientTopicManager) AddSink(ctx context.Context, abort <-chan struct{}, s *sink) {
	t, ok := m[s.NS]
	if !ok {
		t = newClientTopic()
		m[s.NS] = t
	}

	t.ss = append(t.ss, s)
	s.Close = m.newGC(t, s)

	for _, info := range t.seen {
		select {
		case s.ch <- info:
		case <-ctx.Done():
		case <-abort:
		}
	}
}

func (m clientTopicManager) newGC(t *clientTopic, s *sink) func() {
	return func() {
		defer close(s.ch)

		if t.Remove(s) {
			delete(m, s.NS)
		}
	}
}

type clientTopic struct {
	seen map[peer.ID]peer.AddrInfo
	ss   []*sink
}

func newClientTopic() *clientTopic {
	return &clientTopic{
		seen: make(map[peer.ID]peer.AddrInfo),
	}
}

func (t *clientTopic) Consume(abort <-chan struct{}, rec peer.PeerRecord) {
	if _, seen := t.seen[rec.PeerID]; seen {
		return
	}

	info := peer.AddrInfo{ID: rec.PeerID, Addrs: rec.Addrs}
	t.seen[info.ID] = info

	for _, sink := range t.ss {
		sink.Consume(abort, info)
	}
}

func (t *clientTopic) Remove(s *sink) bool {
	for i, x := range t.ss {
		if x == s {
			t.ss[i], t.ss[len(t.ss)-1] = t.ss[len(t.ss)-1], t.ss[i] // mv to last
			t.ss = t.ss[:len(t.ss)-1]                               // pop last
			break
		}
	}

	return len(t.ss) == 0
}
