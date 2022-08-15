package randutil

import (
	"math/rand"
	"sync"
	"time"
	"unsafe"

	"github.com/libp2p/go-libp2p-core/peer"
)

// NewSharedSeeded returns a thread-safe *rand.Rand that has been
// seeded with the current UNIX nanosecond time.
func NewSharedSeeded() *rand.Rand {
	return NewShared(time.Now().UnixNano())
}

// NewShared returns a thread-safe *rand.Rand.
func NewShared(seed int64) *rand.Rand {
	return rand.New(NewSharedSource(seed))
}

// NewSeeded returns a *rand.Rand that has been seeded with the
// current UNIX nanosecond time.
func NewSeeded() *rand.Rand {
	return rand.New(NewSeededSource())
}

// NewSeededSource returns a rand.Source that has been seeded with
// the current UNIX nanosecond time.
func NewSeededSource() rand.Source {
	return rand.NewSource(time.Now().UnixNano())
}

// SourceFromID returns a random source that is seeded with the last
// 8 bytes of the supplied peer.ID.  This provides randomness across
// hosts, while making samplings reproducible within a given host.
//
// This is most often used in the context of random jitters, where
// it presents the advantage of facilitating the analysis of cluster
// logs.  Note that this MUST NOT be used in cases where adversaries
// cannot be allowed to guess the random sequence.
func SourceFromID(id peer.ID) rand.Source {
	if len(id) < 8 {
		panic("invalid peer.ID (length must be >= 8)")
	}

	b := *(*[]byte)(unsafe.Pointer(&id))
	b = b[len(b)-8:]

	return rand.NewSource(*(*int64)(unsafe.Pointer(&b)))
}

// SharedSource is a thread-safe rand.Source64 implementation.
type SharedSource struct {
	mu  sync.Mutex
	src rand.Source64
}

func NewSharedSource(seed int64) *SharedSource {
	return &SharedSource{
		src: rand.NewSource(seed).(rand.Source64),
	}
}

func (s *SharedSource) Uint64() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.src.Uint64()
}

func (s *SharedSource) Int63() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.src.Int63()
}

func (s *SharedSource) Seed(seed int64) {
	s.mu.Lock()
	s.src.Seed(seed)
	s.mu.Unlock()
}
