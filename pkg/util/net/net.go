// Package netutil contains an assortment of network utilities.
package netutil

import (
	"sync"
	"sync/atomic"
	"time"
)

const precision = time.Microsecond * 100

var (
	once  sync.Once
	clock atomic.Value
)

// Time returns the current time, as reported by a global process clock.
// This is more efficient than calling time.Now(). The returned time has
// a precision of 100µs.
func Time() time.Time {
	once.Do(func() {
		clock.Store(time.Now())

		go func() {
			for t := range time.NewTicker(precision).C {
				clock.Store(t)
			}
		}()
	})

	return clock.Load().(time.Time)
}
