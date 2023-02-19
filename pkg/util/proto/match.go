package protoutil

import (
	"path"
	"strings"

	"github.com/coreos/go-semver/semver"
	"github.com/libp2p/go-libp2p/core/protocol"
)

type MatchFunc func(string) (string, bool)

func (f MatchFunc) Match(id protocol.ID) bool {
	_, ok := f(string(id))
	return ok
}

func (f MatchFunc) Then(next MatchFunc) MatchFunc {
	if f == nil {
		return next
	}

	return match(func(s string) (_ string, ok bool) {
		if s, ok = f(s); ok {
			s, ok = match(next)(s)
		}

		return s, ok
	})
}

func Match(ms ...MatchFunc) (f MatchFunc) {
	for _, next := range ms {
		f = f.Then(next)
	}

	return
}

func Exactly(s string) MatchFunc {
	s = clean(s)
	return match(func(proto string) (string, bool) {
		head, tail := pop(proto)
		return tail, head == s
	})
}

func Prefix(prefix protocol.ID) MatchFunc {
	p := clean(string(prefix))
	return match(func(s string) (string, bool) {
		trimmed := strings.TrimPrefix(s, p)
		return trimmed, trimmed != s
	})
}

func Suffix(suffix protocol.ID) (f MatchFunc) {
	sx := clean(string(suffix))
	return match(func(s string) (string, bool) {
		trimmed := strings.TrimSuffix(s, sx)
		return trimmed, trimmed != s
	})
}

// SemVer returns a function that compares the protocol ID with the
// supplied semantic version string.  It returns true iff the major
// version numbers are identical.
//
// SemVer is compliant with the Semantic Versioning 2.0.0 spec.
// https://semver.org/
func SemVer(version string) MatchFunc {
	v := semver.New(clean(version))

	return match(func(s string) (string, bool) {
		head, tail := pop(s)

		sv, err := semver.NewVersion(head)
		if err != nil {
			return s, false
		}

		return tail, v.Major == sv.Major
	})
}

func clean(s string) string {
	return strings.TrimLeft(path.Clean(s), "/.")
}

func match(f func(string) (string, bool)) MatchFunc {
	return func(s string) (string, bool) {
		return f(clean(s))
	}
}

func pop(s string) (string, string) {
	switch ss := strings.SplitN(clean(s), "/", 2); len(ss) {
	case 0:
		return "", ""

	case 1:
		return ss[0], ""

	default:
		return ss[0], ss[1]
	}
}
