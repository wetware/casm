// Package cmdutil contains utilities for CLI commands
package cmdutil

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/urfave/cli/v2"
	logutil "github.com/wetware/casm/internal/util/log"
)

func BindLogger(c *cli.Context) {
	c.Context = logutil.WithLogger(c.Context, logutil.New(c))
}

func BindTimeout(c *cli.Context) (cancel context.CancelFunc) {
	cancel = func() {}
	if d := c.Duration("timeout"); d != 0 {
		c.Context, cancel = context.WithTimeout(c.Context, d)
	}
	return
}

// BindSingals derives a context that expires when the process receives
// any of the following signals from the operating system:
// - SIGINT
// - SIGTERM
// - SIGKILL
func BindSignals(c *cli.Context) {
	c.Context = withSignals(c.Context,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGKILL)
}

// withSignals returns a context that expires when the process receives any of the
// specified signals.
func withSignals(ctx context.Context, sigs ...os.Signal) context.Context {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, sigs...)

	ctx, cancel := context.WithCancel(ctx)
	go func() {
		defer cancel()

		logutil.Logger(ctx).
			WithField("signals", sigs).
			Debug("signals bound")

		sig := <-ch

		logutil.Logger(ctx).
			WithField("signal", sig).
			Debug("signal received")
	}()

	return ctx
}
