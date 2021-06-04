package pex

import (
	"context"
	"encoding/json"
	"time"

	"github.com/libp2p/go-libp2p-core/network"

	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/libp2p/go-libp2p-core/host"
)

const gossipProtocol = "citizen/gossip/0.1"

type Gossip struct {
	Tags   []string
	Gossip []byte
}

type PeersSource interface {
	peers() peer.IDSlice
}

type GossipEngine struct {
	host   host.Host
	source PeersSource

	tick      time.Duration
	ctx       context.Context
	ctxCancel context.CancelFunc
	chDaemon  chan bool

	running bool

	gossipers map[string]Gossiper
}

func NewGossipEngine(host host.Host, source PeersSource) GossipEngine {
	return GossipEngine{host: host, source: source, gossipers: make(map[string]Gossiper)}
}

func (gspe *GossipEngine) Start(tick time.Duration) error {
	if gspe.running {
		return GossipRunningError
	}
	gspe.tick = tick
	gspe.ctx, gspe.ctxCancel = context.WithCancel(context.Background())
	gspe.host.SetStreamHandler(gossipProtocol, func(stream network.Stream) {
		gspe.pull(stream)
	})
	go gspe.cancelDetector(gspe.ctx)
	go gspe.gossipLoop()
	gspe.running = true
	return nil
}

func (gspe *GossipEngine) Stop() {
	gspe.ctxCancel() // this triggers cancelDetector. This is made for being thread-safe
}

type Gossiper interface {
	id() string
	getGossips() []Gossip
	gossipsUpdate(gossips []Gossip)
}

func (gspe *GossipEngine) AddGossiper(gspr Gossiper) error {
	if gspe.running {
		if _, ok := gspe.gossipers[gspr.id()]; ok {
			return DuplicateGossiperError
		}
		gspe.gossipers[gspr.id()] = gspr
		return nil
	}
	return GossipNoRunningError
}

func (gspe *GossipEngine) RemoveGossiper(id string) error {
	if _, ok := gspe.gossipers[id]; !ok {
		return GossiperNotFoundError
	}
	delete(gspe.gossipers, id)
	return nil
}

func (gspe *GossipEngine) gossipLoop() {
	for {
		select {
		case <-gspe.ctx.Done():
			return
		case <-time.After(gspe.tick):
			go gspe.gossip()
		}
	}

}

func (gspe *GossipEngine) gossip() {
	for _, p := range gspe.source.peers() {
		if p != gspe.host.ID() {
			ctx, _ := context.WithCancel(gspe.ctx)
			stream, err := gspe.host.NewStream(ctx, p, gossipProtocol)
			if err != nil {
				continue
			}
			defer stream.Close()
			err = gspe.push(stream)
			if err != nil {
				continue
			}
			gspe.pull(stream)
			return
		}
	}
}

func (gspe *GossipEngine) push(stream network.Stream) error {
	enc := json.NewEncoder(stream) // TODO: Implement correct encoder
	gossips := make([]Gossip, 0)
	for _, gspr := range gspe.gossipers {
		gossips = append(gossips, gspr.getGossips()...)
	}
	return enc.Encode(gossips)
}

func (gspe *GossipEngine) pull(stream network.Stream) {
	dec := json.NewDecoder(stream) // TODO: Implement correct encoder
	gossips := make([]Gossip, 0)
	err := dec.Decode(&gossips)
	if err != nil {
		return
	}
	for _, gspr := range gspe.gossipers {
		go gspr.gossipsUpdate(gossips)
	}
}

func (gspr *GossipEngine) cancelDetector(ctx context.Context) {
	<-ctx.Done()
	gspr.host.RemoveStreamHandler(gossipProtocol)
	gspr.running = false
}
