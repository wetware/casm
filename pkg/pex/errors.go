package pex

import (
	"errors"
	"fmt"
)

var (
	// ErrClosed is returned from an operation performed against a
	// closed PeerExchange or namespace.
	ErrClosed = errors.New("closed")

	// ErrNotFound is returned from an operation performed against
	// a non-existent namespace.
	ErrNotFound = errors.New("not found")

	// ErrNoListenAddrs is returned from 'New' if the supplied host
	// is not accepting peer connections.
	ErrNoListenAddrs = errors.New("host not accepting connections")

	// ErrInvalidRange is returned as a cause in a ValidationError when
	// a field's value falls outside the expected range.
	ErrInvalidRange = errors.New("invalid range")
)

type ValidationError struct {
	Cause   error
	Message string
}

func (err ValidationError) Error() string {
	if err.Cause != nil {
		return fmt.Sprintf("%s: %s", err.Message, err.Cause)
	}

	return err.Message
}

func (err ValidationError) Unwrap() error        { return err.Cause }
func (err ValidationError) Is(target error) bool { return errors.Is(err.Cause, target) }
