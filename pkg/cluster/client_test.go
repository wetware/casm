package cluster

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	routing_api "github.com/wetware/casm/internal/api/routing"
)

func TestHandler(t *testing.T) {
	t.Parallel()
	t.Helper()

	t.Run("Sync", func(t *testing.T) {
		h := newHandler()
		go func() { h.send <- nil }()

		_, _ = h.Next()
		assert.Len(t, h.sync, 1, "should synchronize")
	})

	t.Run("ParamError", func(t *testing.T) {
		h := newHandler()

		errTest := errors.New("test")
		setParam := h.Handler(func(QueryParams) error { return errTest })
		err := setParam(routing_api.View_iter_Params{})
		assert.ErrorIs(t, err, errTest)

		assert.Panics(t, func() { close(h.send) },
			"handler.send should be closed")
		assert.Panics(t, func() { close(h.sync) },
			"handler.sync should be closed")
	})
}
