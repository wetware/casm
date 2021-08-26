package cluster

import (
	"time"
	"unsafe"

	"github.com/libp2p/go-libp2p-core/peer"
)

/*
 * util.go contains unexported utility types
 */

// Nil values are treated as infinite.
func unsafeTimeComparator(a, b interface{}) int {
	switch {
	case a == nil:
		return -1 // N.B.:  treap is a min-heap by default
	case b == nil:
		return 1
	}

	ta := timeCastUnsafe(a)
	tb := timeCastUnsafe(b)

	switch {
	case ta.After(tb):
		return 1
	case ta.Before(tb):
		return -1
	default:
		return 0
	}
}

func unsafePeerIDComparator(a, b interface{}) int {
	switch {
	case a == nil:
		return -1 // N.B.:  treap is a min-heap by default
	case b == nil:
		return 1
	}

	var (
		s1        = stringCastUnsafe(a)
		s2        = stringCastUnsafe(b)
		min, diff int
	)

	if min = len(s2); len(s1) < len(s2) {
		min = len(s1)
	}

	for i := 0; i < min && diff == 0; i++ {
		diff = int(s1[i]) - int(s2[i])
	}

	if diff == 0 {
		diff = len(s1) - len(s2)
	}

	switch {
	case diff < 0:
		diff = -1
	case diff > 0:
		diff = 1
	}

	return diff
}

func timeCastUnsafe(v interface{}) time.Time {
	return *(*time.Time)((*ifaceWords)(unsafe.Pointer(&v)).data)
}

func stringCastUnsafe(v interface{}) string {
	return *(*string)((*ifaceWords)(unsafe.Pointer(&v)).data)
}

func idCastUnsafe(v interface{}) peer.ID {
	return *(*peer.ID)((*ifaceWords)(unsafe.Pointer(&v)).data)
}

func recordCastUnsafe(v interface{}) peerRecord {
	return *(*peerRecord)((*ifaceWords)(unsafe.Pointer(&v)).data)
}

// ifaceWords is interface{} internal representation.
type ifaceWords struct {
	typ  unsafe.Pointer
	data unsafe.Pointer
}
