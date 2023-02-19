package casm_test

import (
	"testing"

	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/stretchr/testify/assert"
	casm "github.com/wetware/casm/pkg"
)

func TestProto(t *testing.T) {
	t.Parallel()

	const ns = "test"
	matcher := casm.NewMatcher(ns)
	proto := casm.Subprotocol(ns)

	assert.True(t, matcher.Match(proto),
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

func TestMatchLz4(t *testing.T) {
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
			proto: "/foo/bar/lz4",
			match: true,
		},
		{
			name:  "packed",
			proto: "/foo/bar/packed/lz4",
			match: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.match {
				assert.True(t, casm.MatchLz4(tt.proto),
					"%s should match %s", tt.name, tt.proto)
			} else {
				assert.False(t, casm.MatchLz4(tt.proto),
					"%s should not match %s", tt.name, tt.proto)
			}
		})
	}
}

func TestLz4MatchPacked(t *testing.T) {
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
			proto: "/foo/bar/lz4",
		},
		{
			name:  "packed",
			proto: "/foo/bar/packed/lz4",
			match: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := casm.Lz4Stream{Stream: MockStream{tt.proto}}
			if tt.match {
				assert.True(t, casm.MatchPacked(s.Protocol()),
					"%s should match %s", tt.name, tt.proto)
			} else {
				assert.False(t, casm.MatchPacked(s.Protocol()),
					"%s should not match %s", tt.name, tt.proto)
			}
		})
	}
}

type MockStream struct {
	protocol.ID
}

func (s MockStream) Protocol() protocol.ID {
	return s.ID
}

func (s MockStream) Read(b []byte) (int, error) {
	return 0, nil
}

func (s MockStream) Write(b []byte) (int, error) {
	return 0, nil
}

func (s MockStream) Close() error {
	return nil
}
