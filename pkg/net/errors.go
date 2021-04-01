package net

import (
	"errors"
)

var (
	// ErrNoPeers is a sentinel error used to signal that a reboot
	// has failed because there were no peers in the PeerStore.
	ErrNoPeers = errors.New("no peers")
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
