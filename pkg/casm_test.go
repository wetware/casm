package casm_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	casm "github.com/wetware/casm/pkg"
)

func TestProto(t *testing.T) {
	t.Parallel()

	const ns = "test"
	match := casm.NewMatcher(ns)
	proto := casm.Subprotocol(ns)

	assert.True(t, match(string(proto)),
		"matcher should match subprotocol")
}
