package pex

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"

	"github.com/stretchr/testify/assert"

	"github.com/libp2p/go-libp2p-core/network"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/host"
)

type peerSource struct {
	store peerstore.Peerstore
}

func (ps *peerSource) peers() peer.IDSlice {
	return ps.store.Peers()
}

func TestGossipStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cluster := newCluster(t, ctx, 2)

	var (
		mu        sync.Mutex
		gspAmount = 0
	)

	cluster[1].SetStreamHandler(gossipProtocol, func(stream network.Stream) {
		defer stream.Close()
		mu.Lock()
		gspAmount += 1
		mu.Unlock()
	})

	gspe := NewGossipEngine(cluster[0], &peerSource{cluster[0].Peerstore()})
	tick := time.Millisecond
	const tickN = 2
	err := gspe.Start(tick)
	if err != nil {
		t.Error(err)
	}

	<-time.After(tick * (tickN + 10)) // +10 because streamHandler takes a lot of time for receiving calls

	mu.Lock()
	assert.GreaterOrEqual(t, gspAmount, tickN)
	assert.LessOrEqual(t, gspAmount, tickN+10)
	mu.Unlock()

	err = gspe.Start(tick)
	if err == nil {
		t.Errorf("error expected because gossiper is already running")
	}

	// TODO: check that order of source.peers() is respected
}

func TestGossipStop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cluster := newCluster(t, ctx, 2)

	gspe := NewGossipEngine(cluster[0], &peerSource{cluster[0].Peerstore()})
	tick := time.Millisecond
	const tickN = 2
	err := gspe.Start(tick)
	if err != nil {
		t.Error(err)
	}
	<-time.After(tick * (tickN + 10)) // +10 because streamHandler takes a lot of time for receiving calls

	var (
		mu        sync.Mutex
		gspAmount = 0
	)

	cluster[1].SetStreamHandler(gossipProtocol, func(stream network.Stream) {
		defer stream.Close()
		mu.Lock()
		gspAmount += 1
		mu.Unlock()
	})

	mu.Lock()
	assert.Equal(t, gspAmount, 0)
	mu.Unlock()

	err = gspe.Start(tick)
	assert.ErrorIs(t, err, GossipRunningError)
}

func TestAddRemoveGossiper(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cluster := newCluster(t, ctx, 2)

	tick := time.Millisecond
	gspe := NewGossipEngine(cluster[0], &peerSource{cluster[0].Peerstore()})

	gsprI := basicGossiper{gsprId: "0"}
	gsprJ := basicGossiper{gsprId: "1"}

	err := gspe.AddGossiper(&gsprI)
	assert.ErrorIs(t, err, GossipNoRunningError) // try to add gossiper to non-running engine

	err = gspe.Start(tick)
	assert.NoError(t, err)

	err = gspe.AddGossiper(&gsprI) // add gossiper to running engine
	assert.NoError(t, err)

	err = gspe.AddGossiper(&gsprI)
	assert.ErrorIs(t, err, DuplicateGossiperError) // add duplicate gossiper

	err = gspe.AddGossiper(&gsprJ) // add second gossiper
	assert.NoError(t, err)

	err = gspe.RemoveGossiper(gsprI.id()) // remove add gossiper
	assert.NoError(t, err)

	err = gspe.RemoveGossiper(gsprI.id())
	assert.ErrorIs(t, err, GossiperNotFoundError) // remove previously removed gossiper

	err = gspe.AddGossiper(&gsprI) // add removed gossiper
	assert.NoError(t, err)
}

func TestGossip(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tick := time.Second
	const tickN = 2
	gspes := newStartedGossipEngines(t, ctx, 2, tick)

	gsprI := newBasicGossip("0")
	gsprJ := newBasicGossip("1")
	err := gspes[0].AddGossiper(&gsprI)
	err = gspes[1].AddGossiper(&gsprJ)
	assert.NoError(t, err)
	assert.NoError(t, err)

	for _, gossip := range gsprI.getGossips() {
		assert.NotContains(t, gsprJ.ReceivedGossips(), gossip)
	}

	<-time.After(tick * (tickN + 1))

	for _, gossip := range gsprI.getGossips() {
		assert.Contains(t, gsprJ.ReceivedGossips(), gossip)
	}

}

func TestRemoveGossiper(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tick := time.Second
	const tickN = 2
	gspes := newStartedGossipEngines(t, ctx, 2, tick)

	gsprI := newBasicGossip("0")
	gsprJ := newBasicGossip("1")
	err := gspes[0].AddGossiper(&gsprI)
	err = gspes[1].AddGossiper(&gsprJ)
	assert.NoError(t, err)
	assert.NoError(t, err)

	<-time.After(tick * (tickN + 1))
	err = gspes[0].RemoveGossiper(gsprI.id())
	assert.NoError(t, err)

	rcvGossips1 := gsprJ.ReceivedGossips()
	<-time.After(tick * (tickN + 1))
	assert.Equal(t, len(rcvGossips1), len(gsprJ.ReceivedGossips()))
	for _, gossip := range gsprJ.ReceivedGossips() {
		assert.Contains(t, rcvGossips1, gossip)
	}
}

type basicGossiper struct {
	gsprId      string
	recvGossips []Gossip
}

func newBasicGossip(id string) basicGossiper {
	return basicGossiper{id, make([]Gossip, 0)}
}

func (gspr *basicGossiper) id() string {
	return gspr.gsprId
}

func (gspr *basicGossiper) getGossips() []Gossip {
	gossips := make([]Gossip, 1)
	gossips[0] = Gossip{Tags: []string{}, Gossip: []byte(fmt.Sprintf("gossip %s", gspr.id()))}
	return gossips
}

func (gspr *basicGossiper) gossipsUpdate(gossips []Gossip) {
	gspr.recvGossips = append(gspr.recvGossips, gossips...)
}

func (gspr basicGossiper) ReceivedGossips() []Gossip {
	return gspr.recvGossips
}

func newStartedGossipEngines(t *testing.T, ctx context.Context, amount int, tick time.Duration) []GossipEngine {
	cluster := newCluster(t, ctx, amount)
	gspes := make([]GossipEngine, len(cluster))

	for i := 0; i < len(gspes); i++ {
		gspes[i] = NewGossipEngine(cluster[i], &peerSource{cluster[i].Peerstore()})
		err := gspes[i].Start(tick)
		if err != nil {
			t.Error(err)
		}
	}
	return gspes
}

func newCluster(t *testing.T, ctx context.Context, amount int) []host.Host {
	cluster := make([]host.Host, amount)
	for i := 0; i < amount; i++ {
		opts := []libp2p.Option{
			libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", 49155+i)),
		}
		h, err := libp2p.New(ctx, opts...)
		if err != nil {
			t.Error(err)
		}
		cluster[i] = h
	}
	for i := 0; i < amount; i++ {
		for j := 0; j < amount; j++ {
			if i != j {
				cluster[i].Peerstore().AddAddrs(cluster[j].ID(), cluster[j].Addrs(), 1*time.Minute) // Be careful with TTL: set high enough value!
			}
		}
	}
	return cluster
}
