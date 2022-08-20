package casm_test

import (
	"testing"

	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/stretchr/testify/assert"
	casm "github.com/wetware/casm/pkg"
)

func TestProto(t *testing.T) {
	t.Parallel()

	const ns = "test"
	matcher := casm.NewMatcher(ns)
	proto := casm.Subprotocol(ns)

	assert.True(t, matcher.MatchProto(proto),
		"matcher should match subprotocol")
}

func TestMatchPacked(t *testing.T) {
	t.Parallel()
	t.Helper()

	for _, tt := range []struct {
		name  string
		proto protocol.ID
		match bool
	}{
		{
			name:  "raw",
			proto: "/foo/bar",
		},
		{
			name:  "packed",
			proto: "/foo/bar/packed",
			match: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.match {
				assert.True(t, casm.MatchPacked(tt.proto),
					"%s should match %s", tt.name, tt.proto)
			} else {
				assert.False(t, casm.MatchPacked(tt.proto),
					"%s should not match %s", tt.name, tt.proto)
			}
		})
	}
}
