package net

import (
	"errors"
)

var (
	// ErrNoPeers indicates an operation failed because there were
	// reachable peers in the cluster.
	ErrNoPeers = errors.New("no peers")

	// ErrConnClosed is returned when a lease/evict operation fails
	// due to a closed RPC connection.
	ErrConnClosed = errors.New("connection closed")

	errEdgeExists = errors.New("edge exists")
)

// JoinError is returned upon failing to join an overlay.
type JoinError struct {
	Report, Cause error
}

func (err JoinError) Error() string {
	if err.Report != nil {
		return err.Report.Error()
	}

	return err.Cause.Error()
}

func (err JoinError) Unwrap() error { return err.Cause }

func (err JoinError) Is(target error) bool {
	return errors.Is(err.Cause, target) || errors.Is(err.Report, target)
}
