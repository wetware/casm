package boot

import (
	"fmt"

	ma "github.com/multiformats/go-multiaddr"
)

/*
 * util.go contains unexported utilities
 */

// NewTransport parses a multiaddr and returns the corresponding transport,
// if it exists.  Note that addresses MUST start with a protocol identifier
// such as '/multicast'.
func NewTransport(m ma.Multiaddr) (_ Transport, err error) {
	switch h := head(m); h.Protocol().Code {
	case P_MCAST:
		if m, err = ma.NewMultiaddrBytes(h.RawValue()); err != nil {
			return nil, err
		}

		return NewMulticastUDP(m)

	default:
		return nil, fmt.Errorf("invalid boot protocol '%s'", h.String())
	}
}

func head(m ma.Multiaddr) (c ma.Component) {
	ma.ForEach(m, func(cc ma.Component) bool {
		c = cc
		return false
	})
	return
}

type breaker struct{ Err error }

func (b breaker) Do(f func()) {
	if b.Err == nil {
		f()
	}
}
