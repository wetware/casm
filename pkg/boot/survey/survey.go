package survey

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"time"

	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/libp2p/go-libp2p-core/record"

	"github.com/lthibault/log"

	"github.com/wetware/casm/pkg/boot/socket"
)

var ErrClosed = errors.New("closed")

// Surveyor discovers peers through a surveyor/respondent multicast
// protocol.
type Surveyor struct {
	log    log.Logger
	lim    *socket.RateLimiter
	sock   *socket.Socket
	host   host.Host
	cache  *socket.RecordCache
	done   <-chan struct{}
	cancel context.CancelFunc
}

// New surveyor.  The supplied PacketConn SHOULD be bound to a multicast
// group.  Use of JoinMulticastGroup to construct conn is RECOMMENDED.
func New(h host.Host, conn net.PacketConn, opt ...Option) *Surveyor {
	ctx, cancel := context.WithCancel(context.Background())

	s := &Surveyor{
		host:   h,
		done:   ctx.Done(),
		cancel: cancel,
	}

	for _, option := range withDefaults(opt) {
		option(s)
	}

	s.sock = socket.New(conn, s.lim, socket.Protocol{
		HandleError:   s.socketErrHandler(ctx),
		HandleRequest: s.requestHandler(ctx),
		Validate:      socket.BasicValidator(h.ID()),
		Cache:         s.cache,
	})

	return s
}

func (s *Surveyor) Close() error {
	s.cancel()
	return s.sock.Close()
}

func (s *Surveyor) socketErrHandler(ctx context.Context) func(err error) {
	return func(err error) {
		if ctx.Err() == nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				s.log.Debug("read timeout")
				return
			}

			s.log.WithError(err).Error("socket error")
		}
	}
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
			e, err := s.cache.LoadResponse(s.sealer(), ns)
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
	var opts = discovery.Options{Ttl: peerstore.TempAddrTTL}
	if err := opts.Apply(opt...); err != nil {
		return 0, err
	}

	return opts.Ttl, s.sock.Track(ctx, s.host, ns, opts.Ttl)
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
