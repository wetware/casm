package view_test

// func TestView(t *testing.T) {
// 	t.Parallel()

// 	rt := routing.New(time.Now())
// 	_ = rt.Upsert(&record{id: newPeerID()})

// 	s := cluster.View{Query: rt.NewQuery()}
// 	view := s.Client()

// }

// type record struct {
// 	once sync.Once
// 	id   peer.ID
// 	seq  uint64
// 	ins  uint32
// 	host string
// 	meta routing.Meta
// 	ttl  time.Duration
// }

// func (r *record) init() {
// 	r.once.Do(func() {
// 		if r.id == "" {
// 			r.id = newPeerID()
// 		}

// 		if r.host == "" {
// 			r.host = newPeerID().String()[:16]
// 		}

// 		if r.ins == 0 {
// 			r.ins = rand.Uint32()
// 		}
// 	})
// }

// func (r *record) Peer() peer.ID {
// 	r.init()
// 	return r.id
// }

// func (r *record) Seq() uint64 { return r.seq }

// func (r *record) Host() (string, error) {
// 	r.init()
// 	return r.host, nil
// }

// func (r *record) Instance() uint32 {
// 	r.init()
// 	return r.ins
// }

// func (r *record) TTL() time.Duration {
// 	if r.init(); r.ttl == 0 {
// 		return time.Second
// 	}

// 	return r.ttl
// }

// func (r *record) Meta() (routing.Meta, error) { return r.meta, nil }

// func newPeerID() peer.ID {
// 	sk, _, err := crypto.GenerateEd25519Key(rand)
// 	if err != nil {
// 		panic(err)
// 	}

// 	id, err := peer.IDFromPrivateKey(sk)
// 	if err != nil {
// 		panic(err)
// 	}

// 	return id
// }
