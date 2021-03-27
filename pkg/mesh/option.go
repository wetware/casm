package mesh

import (
	"zombiezen.com/go/capnproto2/server"
)

// Option type for Neighborhood.
type Option func(*Neighborhood)

// WithServerPolicy .
func WithServerPolicy(p *server.Policy) Option {
	if p == nil {
		p = &server.Policy{}
	}

	return func(n *Neighborhood) {
		n.policy = p
	}
}

// WithParams .
func WithParams(k, overflow uint8) Option {
	if k == 0 {
		k = 5
	}

	if overflow == 0 {
		overflow = 2
	}

	return func(n *Neighborhood) {
		n.k = k
		n.overflow = overflow
	}
}

func withDefaults(opt []Option) []Option {
	return append([]Option{
		WithParams(0, 0),
		WithServerPolicy(nil),
	}, opt...)
}
