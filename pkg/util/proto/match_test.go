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
		match         MatchFunc
		input         string
		expectNoMatch bool
	}{
		{
			name:  "Exactly/match",
			match: Exactly("foo"),
			input: "/bar/foo/",
		},
		{
			name:          "Exactly/reject",
			match:         Exactly("foo"),
			input:         "/foo/bar/",
			expectNoMatch: true,
		},
		{
			name:  "Prefix/match",
			match: Prefix("/foo/bar/"),
			input: "/foo/bar/baz/qux",
		},
		{
			name:          "Prefix/reject",
			match:         Prefix("/foo/bar/"),
			input:         "/bar/foo/baz/qux/",
			expectNoMatch: true,
		},
		{
			name:  "SemVer/match",
			match: SemVer("1.1.0-beta.1"),
			input: "/1.1.5/",
		},
		{
			name:  "SemVer/match/minor",
			match: SemVer("1.1.0-beta.1"),
			input: "/1.0.0/",
		},
		{
			name:          "SemVer/reject",
			match:         SemVer("1.1.0-beta.1"),
			input:         "/foobar/2.1.0/",
			expectNoMatch: true,
		},
		{
			name:          "SemVer/reject/minor",
			match:         SemVer("1.1.0-beta.1"),
			input:         "/1.2.1/",
			expectNoMatch: true,
		},
		{
			name:          "SemVer/reject/malformed",
			match:         SemVer("1.1.0-beta.1"),
			input:         "/not a semver string/",
			expectNoMatch: true,
		},
		{
			name: "MatchComplex",
			match: Match(
				Prefix("ww"),
				SemVer("1.5.1"),
				Exactly("ns"),
				Exactly("rpc")),
			input: "/ww/1.0.0/ns/rpc/",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if match := tt.match(tt.input); tt.expectNoMatch {
				assert.False(t, match,
					"should not match '%s'", tt.input)
			} else {
				assert.True(t, match,
					"should match '%s'", tt.input)
			}
		})
	}
}
