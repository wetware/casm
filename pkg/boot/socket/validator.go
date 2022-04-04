package socket

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/lthibault/log"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/record"
)

type Validator func(*record.Envelope, *Record) error

var (
	// ErrIgnore causes a message to be dropped silently.
	// It is typically used when filtering out messgaes that
	// originate from the local host.
	ErrIgnore = errors.New("ignore")
)

type ValidationError struct {
	From  net.Addr
	Cause error
}

func (ve ValidationError) Loggable() map[string]interface{} {
	return map[string]interface{}{
		"from":  ve.From,
		"error": ve.Cause,
	}
}

func (ve ValidationError) Error() string {
	return fmt.Sprintf("validation failed: %s", ve.Cause)
}

func (ve ValidationError) Is(err error) bool {
	return errors.Is(err, ve.Cause)
}

func (ve ValidationError) Unwrap() error {
	return ve.Cause
}

// BasicValidator returns a validator that checks that the
// host corresponding to id is not the packet's originator,
// and that that the envelope was signed by the peer whose
// ID appears in the packet.
func BasicValidator(id peer.ID) Validator {
	return func(e *record.Envelope, r *Record) error {
		peer, err := r.PeerID()
		if err != nil {
			return err
		}

		// originates from local host?
		if peer == id {
			return ErrIgnore
		}

		// envelope was signed by peer?
		if !peer.MatchesPublicKey(e.PublicKey) {
			err = errors.New("envelope not signed by peer")
		}

		return err
	}
}

func BasicErrHandler(ctx context.Context, log log.Logger) func(error) {
	return func(err error) {
		if err == nil || errors.Is(err, ErrIgnore) || ctx.Err() != nil {
			return
		}

		switch e := err.(type) {
		case net.Error:
			if errors.Is(err, net.ErrClosed) {
				log.Error(err)
			} else if e.Timeout() {
				log.Debug("read timeout")
			} else {
				log.WithError(err).Error("network error")
			}

		case ValidationError:
			log.With(e).Debug("validation failed")

		default:
			log.WithError(err).Error("socket error")
		}
	}
}
