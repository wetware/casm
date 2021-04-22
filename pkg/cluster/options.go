package cluster

import "github.com/lthibault/log"

// Option type for cluster.
type Option func(*Cluster)

// WithLogger sets the logger for the cluster.
//
// If l == nil, a default logger is used.
func WithLogger(l log.Logger) Option {
	if l == nil {
		l = log.New()
	}

	return func(c *Cluster) {
		c.log = l.WithField("type", "casm.cluster")
	}
}

func withDefaults(opt []Option) []Option {
	return append([]Option{
		WithLogger(nil),
	}, opt...)
}
