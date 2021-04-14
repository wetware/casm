package net_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wetware/casm/pkg/net"
)

func TestJoinError(t *testing.T) {
	t.Parallel()

	t.Run("Report", func(t *testing.T) {
		t.Parallel()

		err := net.JoinError{
			Report: errors.New("report"),
			Cause:  errors.New("cause"),
		}
		require.EqualError(t, err, "report")

		require.EqualError(t, net.JoinError{Cause: errors.New("cause")}, "cause")
	})

	t.Run("Is", func(t *testing.T) {
		t.Parallel()

		err := net.JoinError{
			Report: net.ErrNoPeers,
			Cause:  net.ErrConnClosed,
		}

		require.ErrorIs(t, err, net.ErrNoPeers)
		require.ErrorIs(t, err, net.ErrConnClosed)
	})

	t.Run("Unwrap", func(t *testing.T) {
		t.Parallel()

		err := net.JoinError{Cause: errors.New("test")}
		require.EqualError(t, err.Unwrap(), "test")
	})
}
