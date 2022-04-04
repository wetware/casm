package survey

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/libp2p/go-libp2p-core/record"
	"go.uber.org/multierr"

	"github.com/lthibault/log"

	"github.com/wetware/casm/pkg/boot/socket"
	"github.com/wetware/casm/pkg/util/tracker"
)

var ErrClosed = errors.New("closed")

// Surveyor discovers peers through a surveyor/respondent multicast
// protocol.
type Surveyor struct {
	log     log.Logger
	host    host.Host
	once    sync.Once
	tracker *tracker.HostAddrTracker

	lim   *socket.RateLimiter
	sock  *socket.Socket
	cache *socket.RequestResponseCache

	done   <-chan struct{}
	cancel context.CancelFunc
}

// New surveyor.  The supplied PacketConn SHOULD be bound to a multicast
// group.  Use of JoinMulticastGroup to construct conn is RECOMMENDED.
func New(h host.Host, conn net.PacketConn, opt ...Option) *Surveyor {
	ctx, cancel := context.WithCancel(context.Background())

	s := &Surveyor{
		host:    h,
		done:    ctx.Done(),
		cancel:  cancel,
		tracker: tracker.New(h),
	}

	for _, option := range withDefaults(opt) {
		option(s)
	}

	s.tracker.AddCallback(func(*peer.PeerRecord) { s.cache.Reset() })

	s.sock = socket.New(conn, socket.Protocol{
		Validate:      socket.BasicValidator(h.ID()),
		HandleError:   socket.BasicErrHandler(ctx, s.log),
		HandleRequest: s.requestHandler(ctx),
		// RateLimiter:   s.lim,  // FIXME:  blocks reads when waiting to write
	})
	s.sock.Start()

	return s
}

func (s *Surveyor) Close() error {
	s.cancel()
	return multierr.Combine(s.tracker.Close(), s.sock.Close())
}

func (s *Surveyor) requestHandler(ctx context.Context) func(socket.Request, net.Addr) {
	return func(r socket.Request, addr net.Addr) {
		id, err := r.From()
		if err != nil {
			s.log.WithError(err).Debug("invalid ID in request packet")
			return
		}

		// request comes from itself?
		if id == s.host.ID() {
			return
		}

		// distance too large?
		if s.ignore(id, r.Distance()) {
			return
		}

		ns, err := r.Namespace()
		if err != nil {
			s.log.WithError(err).Debug("invalid namespace in request packet")
			return
		}

		if s.sock.Tracking(ns) {
			e, err := s.cache.LoadResponse(s.sealer(), s.tracker, ns)
			if err != nil {
				s.log.WithError(err).Error("error loading response from cache")
				return
			}

			// Send unicast response.
			if err = s.sock.Send(ctx, e, addr); err != nil {
				s.log.WithError(err).Debug("error sending unicast response")
			}
		}
	}
}

func (s *Surveyor) ignore(id peer.ID, d uint8) bool {
	return xor(s.host.ID(), id)>>uint32(d) != 0
}

func (s *Surveyor) Advertise(ctx context.Context, ns string, opt ...discovery.Option) (time.Duration, error) {
	var (
		err  error
		opts = discovery.Options{Ttl: peerstore.TempAddrTTL}
	)
	if err = opts.Apply(opt...); err != nil {
		return 0, err
	}

	if s.once.Do(func() { err = s.tracker.Ensure(ctx) }); err != nil {
		return 0, err
	}

	s.sock.Track(ctx, s.host, ns, opts.Ttl)

	return opts.Ttl, nil
}

func (s *Surveyor) FindPeers(ctx context.Context, ns string, opt ...discovery.Option) (<-chan peer.AddrInfo, error) {
	opts := discovery.Options{Limit: 1}
	if err := opts.Apply(opt...); err != nil {
		return nil, err
	}

	e, err := s.cache.LoadSurveyRequest(s.sealer(), s.host.ID(), ns, distance(opts))
	if err != nil {
		return nil, err
	}

	out, cancel := s.sock.Subscribe(ns, opts.Limit)
	go func() {
		defer cancel()

		select {
		case <-ctx.Done():
		case <-s.done:
		}
	}()

	// Send multicast request.
	if err := s.sock.Send(ctx, e, s.sock.LocalAddr()); err != nil {
		cancel()
		return nil, err
	}

	return out, nil
}

func (s *Surveyor) sealer() socket.Sealer {
	return func(r record.Record) (*record.Envelope, error) {
		return record.Seal(r, privkey(s.host))
	}
}

func privkey(h host.Host) crypto.PrivKey {
	return h.Peerstore().PrivKey(h.ID())
}

func xor(id1, id2 peer.ID) uint32 {
	xored := make([]byte, 4)
	for i := 0; i < 4; i++ {
		xored[i] = id1[len(id1)-i-1] ^ id2[len(id2)-i-1]
	}

	return binary.BigEndian.Uint32(xored)
}
