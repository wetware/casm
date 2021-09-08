package boot

import "github.com/lthibault/log"

// Option is a generic option type for bootstrap-discovery constructors.
type Option func(interface {
	SetOption(interface{}) error
}) error

// WithLogger sets the logger.
func WithLogger(l log.Logger) Option {
	if l == nil {
		l = log.New()
	}

	return withOption(l)
}

func withOption(v interface{}) Option {
	return func(b interface {
		SetOption(interface{}) error
	}) error {
		return b.SetOption(v)
	}
}
