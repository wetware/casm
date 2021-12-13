package routing

import (
	"github.com/libp2p/go-libp2p-core/peer"
)

/*
 * util.go contains unexported utility types
 */

func pidComparator(a, b interface{}) int {
	switch {
	case a == nil:
		return -1 // N.B.:  treap is a min-heap by default
	case b == nil:
		return 1
	}

	var (
		s1        = a.(peer.ID)
		s2        = b.(peer.ID)
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
