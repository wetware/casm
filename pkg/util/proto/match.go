package protoutil

import (
	"path"
	"strings"

	"github.com/coreos/go-semver/semver"
	"github.com/libp2p/go-libp2p-core/protocol"
)

type MatchFunc func(string) bool

func (f MatchFunc) Then(next MatchFunc) MatchFunc {
	if next == nil {
		return f
	}

	return match(func(s string) (ok bool) {
		if ok = f(s); ok {
			ok = next(path.Dir(s)) // pop last path elem
		}
		return
	})
}

func Match(ms ...MatchFunc) (f MatchFunc) {
	for _, next := range ms {
		f = next.Then(f)
	}

	return
}

func Exactly(s string) MatchFunc {
	s = clean(s)
	return match(func(proto string) bool {
		return path.Base(proto) == s
	})
}

func Prefix(prefix protocol.ID) MatchFunc {
	p := clean(string(prefix))
	return match(func(s string) bool {
		return strings.HasPrefix(s, p)
	})
}

func SemVer(version string) MatchFunc {
	v := semver.New(clean(version))

	return match(func(s string) bool {
		sv, err := semver.NewVersion(path.Base(s))
		if err != nil {
			return false
		}

		return v.Major == sv.Major && v.Minor >= sv.Minor
	})
}

func clean(s string) string {
	return strings.TrimLeft(path.Clean(s), "/.")
}

func match(f func(string) bool) MatchFunc {
	return func(s string) bool {
		return f(clean(s))
	}
}
