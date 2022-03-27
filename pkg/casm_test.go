package casm_test

import (
	"testing"

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
