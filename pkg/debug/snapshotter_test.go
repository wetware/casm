package debug_test

import (
	"context"
	"runtime/pprof"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wetware/casm/pkg/debug"
)

func TestSnapshotter(t *testing.T) {
	t.Parallel()
	t.Helper()

	t.Run("ProfileIsNil", func(t *testing.T) {
		t.Parallel()

		b, err := debug.SnapshotServer{}.
			Snapshotter().
			Snapshot(context.Background(), 0)
		assert.ErrorIs(t, err, debug.ErrProfileNotFound,
			"should return error when profile is nil")
		assert.Nil(t, b, "should not return data")
	})

	t.Run("ProfileIsSet", func(t *testing.T) {
		t.Parallel()

		b, err := debug.SnapshotServer{Profile: pprof.Lookup("heap")}.
			Snapshotter().
			Snapshot(context.Background(), 0)
		assert.NoError(t, err, "snapshot should succeed")
		assert.NotNil(t, b, "should return data")
	})
}
