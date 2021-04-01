// Package net implements an overlay network
package net

import (
	"context"
	"math/rand"
	"time"

	"github.com/jbenet/goprocess"
	"github.com/libp2p/go-eventbus"
	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	syncutil "github.com/lthibault/util/sync"
)

var r = rand.New(rand.NewSource(time.Now().UnixNano()))

type Overlay struct {
	log Logger

	ns string
	n  neighborhood
	h  host.Host

	stat statStore
	proc goprocess.Process
}

// New network overlay
func New(h host.Host, opt ...Option) (*Overlay, error) {
	o := &Overlay{h: h}
	for _, option := range withDefaults(opt) {
		option(o)
	}

	return o, o.setup()
}

// Stat returns the current state of the overlay network.
func (o *Overlay) Stat() Stat { return o.stat.Load() }

// Loggable representation of the neighborhood
func (o *Overlay) Loggable() map[string]interface{} {
	return map[string]interface{}{
		"type": "casm.net.overlay",
		"id":   o.h.ID(),
		"ns":   o.ns,
	}
}

func (o *Overlay) Join(ctx context.Context, d discovery.Discoverer, opt ...discovery.Option) error {
	peers, err := d.FindPeers(ctx, o.ns, opt...)
	if err != nil {
		return err
	}

	var b syncutil.Breaker
	for info := range peers {
		b.Go(o.call(ctx, info, (*joiner)(o)))
	}

	if err = b.Wait(); err != nil {
		return JoinError{Report: ErrNoPeers, Cause: err}
	}

	return nil
}

func (o *Overlay) call(ctx context.Context, info peer.AddrInfo, rpc method) func() error {
	return func() error { return rpc.Call(ctx, info) }
}

func (o *Overlay) Leave(ctx context.Context) error {
	go o.proc.CloseAfterChildren()

	select {
	case <-o.proc.Closed():
		return o.proc.Err()
	case <-ctx.Done():
		return ctx.Err()
	}
}

// FindPeers in the overlay by performing a random walk.  The ns value
// is ignored.  The discovery.TTL option is not supported and returns
// an error.
//
// FindPeers satisfies discovery.Discoverer, making it possible to pass
// an overlay to its own Join method.
func (o *Overlay) FindPeers(ctx context.Context, ns string, opt ...discovery.Option) (<-chan peer.AddrInfo, error) {
	var (
		options discovery.Options
		ch      = make(chan peer.AddrInfo, options.Limit)
	)

	if err := options.Apply(defaultSampleOpt(ctx, opt)...); err != nil {
		return nil, err
	}

	go func() {
		defer close(ch)

		ctx, cancel := context.WithTimeout(ctx, options.Ttl)
		defer cancel()

		ns := o.stat.Load().View().Shuffle()

		var j syncutil.Join
		for i := 0; i < options.Limit; i++ {
			peer := ns[i%len(ns)]
			j.Go(o.call(ctx, peer, sampleMethod{d: o.h, opt: options, ch: ch}))
		}

		if err := j.Wait(); err != nil {
			o.log.WithError(err).Debug("some samples have failed")
		}
	}()

	return ch, nil
}

func (o *Overlay) setup() error {
	o.stat.Store(statReady)

	procFn, err := o.loop()
	if err != nil {
		return err
	}

	o.proc = o.h.Network().Process().Go(procFn)
	defer o.proc.SetTeardown(o.teardown) // wait until notif/endpts are registered

	o.h.Network().Notify(&o.n)

	for _, e := range o.endpoints() {
		o.h.SetStreamHandler(e.Protocol(), o.logHandler(e))
	}

	return nil
}

func (o *Overlay) teardown() error {
	for _, e := range o.endpoints() {
		o.h.RemoveStreamHandler(e.Protocol())
	}

	o.h.Network().StopNotify(&o.n)
	o.stat.Store(statClosed)

	return nil
}

func (o *Overlay) loop() (goprocess.ProcessFunc, error) {
	fn, events := o.n.loop()

	state, err := o.h.EventBus().Emitter(new(EvtNetState), eventbus.Stateful)
	if err != nil {
		return nil, err
	}

	return func(p goprocess.Process) {
		defer state.Close()
		defer o.stat.Store(statClosing)

		p.Go(fn)

		var s stat
		for ev := range events {
			s.EvtNetState = ev
			o.stat.Store(s)

			if err := state.Emit(ev); err != nil {
				o.log.WithError(err).Fatal("state emitter closed") // unreachable
			}
		}
	}, nil
}

func (o *Overlay) endpoints() []endpoint {
	return []endpoint{
		(*joiner)(o),
		(*handleSample)(o),
	}
}

func (o *Overlay) logHandler(e endpoint) network.StreamHandler {
	return func(s network.Stream) {
		log := o.log.WithStream(s)
		log.Debug("stream received")
		defer log.Debug("stream terminated")
		defer s.Close()

		if err := e.Handle(log, s); err != nil {
			log.WithError(err).Debug("error encountered in handler")
		}
	}
}
