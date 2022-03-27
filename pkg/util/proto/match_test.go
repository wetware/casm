package protoutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMatchers(t *testing.T) {
	t.Parallel()
	t.Helper()

	for _, tt := range []struct {
		name          string
		matcher       MatchFunc
		input         string
		expectNoMatch bool
	}{
		{
			name:    "Exactly/match",
			matcher: Exactly("foo"),
			input:   "/foo/bar/",
		},
		{
			name:          "Exactly/reject",
			matcher:       Exactly("bar"),
			input:         "/foo/bar/",
			expectNoMatch: true,
		},
		{
			name:    "Prefix/match",
			matcher: Prefix("/foo/bar/"),
			input:   "/foo/bar/baz/qux",
		},
		{
			name:          "Prefix/reject",
			matcher:       Prefix("/foo/bar/"),
			input:         "/bar/foo/baz/qux/",
			expectNoMatch: true,
		},
		{
			name:    "Suffix/Match",
			matcher: Suffix("/baz/qux"),
			input:   "/foo/bar/baz/qux",
		},
		{
			name:          "Suffix/reject",
			matcher:       Suffix("/baz/qux/"),
			input:         "/foo/bar/qux/baz/",
			expectNoMatch: true,
		},
		{
			name:    "SemVer/match",
			matcher: SemVer("1.1.0-beta.1"),
			input:   "/1.1.5/",
		},
		{
			name:    "SemVer/match/minor",
			matcher: SemVer("1.1.0-beta.1"),
			input:   "/1.0.0/",
		},
		{
			name:          "SemVer/reject",
			matcher:       SemVer("1.1.0-beta.1"),
			input:         "/foobar/2.1.0/",
			expectNoMatch: true,
		},
		{
			name:          "SemVer/reject/minor",
			matcher:       SemVer("1.1.0-beta.1"),
			input:         "/1.2.1/",
			expectNoMatch: true,
		},
		{
			name:          "SemVer/reject/malformed",
			matcher:       SemVer("1.1.0-beta.1"),
			input:         "/not a semver string/",
			expectNoMatch: true,
		},
		{
			name: "MatchComplex",
			matcher: Match(
				Prefix("ww"),
				SemVer("1.5.1"),
				Exactly("ns"),
				Exactly("rpc")),
			input: "/ww/1.0.0/ns/rpc/",
		},
		{
			name: "Chain",
			matcher: Match(
				Prefix("ww"),
				SemVer("0.0.0")).Then(Exactly("ns")),
			input: "/ww/0.0.0/ns",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if remainder, match := tt.matcher(tt.input); tt.expectNoMatch {
				assert.False(t, match,
					"should not match '%s' (remainder: %s)", tt.input, remainder)
			} else {
				assert.True(t, match,
					"should match '%s' (remainder: %s)", tt.input, remainder)
			}
		})
	}
}
