package pex

import (
	"context"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/libp2p/go-libp2p-core/host"

	"github.com/stretchr/testify/assert"
)

func TestPeerExchange(t *testing.T) {
	// TODO: split into subtests
	var err error
	ctx, _ := context.WithCancel(context.Background())
	cluster := newCluster(t, ctx, 3)
	addrInfoI := *host.InfoFromHost(cluster[0])
	addrInfoJ := *host.InfoFromHost(cluster[1])
	addrInfoK := *host.InfoFromHost(cluster[2])

	gspoI, err := New(cluster[0])
	defer gspoI.Close()
	assert.NoError(t, err)
	assert.Empty(t, peers(gspoI))
	gspoJ, err := New(cluster[1])
	defer gspoJ.Close()
	assert.NoError(t, err)
	assert.Empty(t, peers(gspoJ))

	err = gspoI.Join(ctx, StaticAddrs{addrInfoJ})
	err = gspoJ.Join(ctx, StaticAddrs{addrInfoK})
	assert.NoError(t, err)
	assert.NoError(t, err)

	assert.Contains(t, peers(gspoI), addrInfoJ)
	assert.NotContains(t, peers(gspoI), addrInfoK)
	assert.Contains(t, peers(gspoJ), addrInfoK)
	assert.NotContains(t, peers(gspoJ), addrInfoI)

	<-time.After(defaultTick * 3)

	assert.Len(t, peers(gspoI), 3)
	assert.Len(t, peers(gspoJ), 3)

	assert.Contains(t, peers(gspoI), addrInfoJ)
	assert.Contains(t, peers(gspoI), addrInfoK)
	assert.Contains(t, peers(gspoJ), addrInfoK)
	assert.Contains(t, peers(gspoJ), addrInfoI)

}

func peers(exchange *PeerExchange) []peer.AddrInfo {
	findPeers, err := exchange.FindPeers(context.Background(), "")
	if err != nil {
		panic("error getting peers from PeerExchange")
	}
	infos := make([]peer.AddrInfo, 0)
	for info := range findPeers {
		infos = append(infos, info)
	}
	return infos
}
