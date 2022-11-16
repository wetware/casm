package metrics

import "time"

// Client is the base interface for all metrics collection performed
// by CASM.  Client are distinct from logs in that they are analyzed
// in aggregate, and are typically sampled to improve performance.
type Client interface {
	// Incr is equivalent to Count(bucket, 1).
	Incr(bucket string)

	// Decr is equivalent to Count(bucket, -1).
	Decr(bucket string)

	// Count increments the bucket by number.
	Count(bucket string, n int)

	// Gauge sends the absolute value of number to the given bucket.
	Gauge(bucket string, n int)

	// Timing marks the start of a duration measure.  It is used
	// to measure multiple durations from a single onset, t0.
	Timing(time.Time) Timing

	// Duration sends a time interval to the given bucket.
	// Precision is implementation-dependent, but usually
	// on the order of milliseconds.
	Duration(bucket string, d time.Duration)

	// Histogram sends an histogram value to a bucket.
	Histogram(bucket string, n int)

	// WithPrefix returns a new Metrics instance with 'prefix'
	// appended to the current prefix string, separated by '.'.
	WithPrefix(prefix string) Client

	// Flush writes all buffered metrics.
	Flush()
}

type Timing struct {
	t0 time.Time
	c  Client
}

func NewTiming(c Client, t0 time.Time) Timing {
	return Timing{t0: t0, c: c}
}

func (t Timing) Send(bucket string) {
	t.c.Duration(bucket, t.Duration())
}

func (t Timing) Duration() time.Duration {
	return time.Since(t.t0)
}

type NopClient struct{}

func (NopClient) Incr(string)                    {}
func (NopClient) Decr(string)                    {}
func (NopClient) Count(string, int)              {}
func (NopClient) Gauge(string, int)              {}
func (NopClient) Duration(string, time.Duration) {}
func (NopClient) Timing(t0 time.Time) Timing     { return NewTiming(NopClient{}, t0) }
func (NopClient) Histogram(string, int)          {}
func (NopClient) Flush()                         {}
func (NopClient) WithPrefix(string) Client       { return NopClient{} }
