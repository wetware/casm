package pex

import (
	"bytes"
	"context"
	"encoding/json"
	"sort"
	"sync/atomic"
	"time"

	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/record"

	"github.com/libp2p/go-libp2p-core/host"

	"github.com/libp2p/go-libp2p-core/peer"
)

type PeerExchange struct {
	host       host.Host
	ns         string
	engine     GossipEngine
	atomicView atomic.Value
	viewSize   int
	tick       time.Duration

	ctx       context.Context
	ctxCancel context.CancelFunc
}

func New(h host.Host, opt ...Option) (*PeerExchange, error) {
	pex := &PeerExchange{host: h}
	pex.setView(make(View, 0))
	pex.viewSize = defaultViewSize
	for _, option := range withDefaults(opt) {
		option(pex)
	}

	err := pex.init()
	return pex, err
}

func (pex *PeerExchange) init() error {
	pex.engine = NewGossipEngine(pex.host, pex)
	pex.ctx, pex.ctxCancel = context.WithCancel(context.Background())
	err := pex.engine.Start(pex.tick)
	if err != nil {
		return err
	}
	err = pex.engine.AddGossiper(pex)
	if err != nil {
		return err
	}
	go pex.cancelDetector(pex.ctx)

	return nil
}

func (pex *PeerExchange) Join(ctx context.Context, d discovery.Discoverer, opt ...discovery.Option) error {
	peers, err := d.FindPeers(ctx, pex.ns, opt...)
	if err != nil {
		return err
	}

	for p := range peers {
		pr := peer.PeerRecordFromAddrInfo(p)
		pex.setView(append(pex.view(), pr))
	}
	return nil
}

func (pex *PeerExchange) FindPeers(_ context.Context, _ string, opt ...discovery.Option) (<-chan peer.AddrInfo, error) {
	view := pex.view()
	options := discovery.Options{Limit: len(view)}
	if err := options.Apply(opt...); err != nil {
		return nil, err
	}

	ch := make(chan peer.AddrInfo, min(options.Limit, len(view)))
	for _, info := range view.addrInfos() {
		ch <- info
	}
	close(ch)
	return ch, nil
}

func (pex *PeerExchange) Close() {
	pex.ctxCancel() // this triggers cancelDetector. This is made for being thread-safe
}

func (pex *PeerExchange) cancelDetector(ctx context.Context) {
	<-ctx.Done()
	_ = pex.engine.RemoveGossiper(pex.id())
	pex.engine.Stop()
}

// next three methods: id(), getGossips(), updateGossips() are implementation of Gossiper interface needed for GossipEngine
var gossiperID = "overlay/gossiper"    // TODO: decide proper gossiper ID
var gossipTag = "overlay/gossip/peers" // TODO: decide proper tag

func (pex *PeerExchange) id() string {
	return gossiperID
}

func (pex *PeerExchange) getGossips() []Gossip {
	gsps := make([]Gossip, 0, 1)
	view := pex.view()
	rawView := make([][]byte, len(view)+1)
	for i := 0; i < len(view); i++ {
		rawRec, err := pex.writeRecord(view[i])
		if err != nil {
			return gsps
		}
		rawView[i] = rawRec
	}
	// add itself
	rawRec, err := pex.writeRecord(&peer.PeerRecord{PeerID: pex.host.ID(), Addrs: pex.host.Addrs()})
	if err != nil {
		return gsps
	}
	rawView[len(rawView)-1] = rawRec

	gossip, err := json.Marshal(rawView)
	if err != nil {
		return gsps
	}
	return append(gsps, Gossip{Tags: []string{gossipTag}, Gossip: gossip})
}

func (pex *PeerExchange) writeRecord(rec *peer.PeerRecord) ([]byte, error) {
	env, err := record.Seal(rec, pex.host.Peerstore().PrivKey(pex.host.ID()))
	if err != nil {
		return nil, err
	}
	return env.Marshal()
}

func (pex *PeerExchange) gossipsUpdate(gossips []Gossip) {
	for _, gossip := range gossips {
		if containsTag(gossip) {
			view, err := pex.readView(gossip.Gossip)
			if err == nil {
				pex.mergeView(view)
			}
		}
	}
}

func containsTag(gossip Gossip) bool {
	for _, tag := range gossip.Tags {
		if tag == gossipTag {
			return true
		}
	}
	return false
}

func (pex *PeerExchange) readView(gossip []byte) (View, error) {
	rawView := make([][]byte, 0)
	err := json.Unmarshal(gossip, &rawView)
	if err != nil {
		return nil, err
	}
	view := make(View, len(rawView))
	for i, env := range rawView {
		rec, err := pex.readRecord(env)
		if err != nil {
			return nil, err
		}
		view[i] = rec
	}
	return view, nil
}

func (pex *PeerExchange) readRecord(env []byte) (*peer.PeerRecord, error) {
	_, rec, err := record.ConsumeEnvelope(env, peer.PeerRecordEnvelopeDomain)
	if err != nil {
		return nil, err
	}
	rec, ok := rec.(*peer.PeerRecord)
	if !ok {

		return nil, err
	}
	return rec.(*peer.PeerRecord), nil
}

func (pex *PeerExchange) mergeView(remoteView View) {
	remoteView.increaseHopCount()
	localView := pex.view()
	source := remoteView[len(remoteView)-1] // by convention gossip source is last record in View
	view := unique(append(localView, remoteView...))
	if peersAreNear(pex.host.ID(), source.PeerID) {
		GlobalAtomicRand.Shuffle(len(view), func(i, j int) {
			view[i], view[j] = view[j], view[i]
		})
	} else {
		sort.Sort(view)
	}
	pex.setView(view[:min(len(view), pex.viewSize)])
}

func peersAreNear(id1 peer.ID, id2 peer.ID) bool {
	return bytes.Compare([]byte(id1), []byte(id2)) == 1 // TODO: implement a more sophisticated comparator?
}

func unique(view View) View {
	uniqueView := make(View, 0)
	temp := make(map[peer.ID]*peer.PeerRecord)
	for _, rec := range view {
		if value, ok := temp[rec.PeerID]; !ok || rec.Seq < value.Seq {
			temp[rec.PeerID] = rec
		}
	}
	for _, rec := range temp {
		uniqueView = append(uniqueView, rec)
	}
	return uniqueView
}

// peers() implements PeersSource interface needed for GossipEngine
func (pex *PeerExchange) peers() peer.IDSlice {
	peers := pex.view().ids()
	GlobalAtomicRand.Shuffle(len(peers), func(i, j int) {
		peers[i], peers[j] = peers[j], peers[i]
	})
	return peers
}

// view() and setView() are getter and setter for atomic View struct
func (pex *PeerExchange) view() View {
	return pex.atomicView.Load().(View)
}

func (pex *PeerExchange) setView(view View) {
	pex.atomicView.Store(view)
}

type View []*peer.PeerRecord

func (v View) Len() int { return len(v) }

func (v View) Less(i, j int) bool { return v[i].Seq < v[j].Seq }

func (v View) Swap(i, j int) { v[i], v[j] = v[j], v[i] }

func (v View) increaseHopCount() {
	for _, rec := range v {
		rec.Seq += 1
	}
}

func (v View) addrInfos() []peer.AddrInfo {
	infos := make([]peer.AddrInfo, len(v))
	for i, pr := range v {
		infos[i] = peer.AddrInfo{pr.PeerID, pr.Addrs}
	}
	return infos
}

func (v View) ids() peer.IDSlice {
	peers := make(peer.IDSlice, len(v))
	for i, pr := range v {
		peers[i] = pr.PeerID
	}
	return peers
}
