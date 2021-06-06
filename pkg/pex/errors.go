package pex

import (
	"errors"
	"fmt"
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
