package vat_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wetware/casm/pkg/vat"
)

func TestProto(t *testing.T) {
	t.Parallel()

	const ns = "test"
	match := vat.NewMatcher(ns)
	proto := vat.Subprotocol(ns)

	assert.True(t, match(string(proto)),
		"matcher should match subprotocol")
}
