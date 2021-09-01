// Package logutil contains shared utilities for configuring loggers from a cli context.
package logutil

import (
	"context"

	"github.com/lthibault/log"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

// New logger from a cli context
func New(c *cli.Context, opt ...log.Option) log.Logger {
	return log.New(append([]log.Option{
		WithLevel(c),
		WithFormat(c),
		withErrWriter(c),
	}, opt...)...)
}

// WithLevel returns a log.Option that configures a logger's level.
func WithLevel(c *cli.Context) (opt log.Option) {
	var level = log.FatalLevel
	defer func() {
		opt = log.WithLevel(level)
	}()

	if c.Bool("trace") {
		level = log.TraceLevel
		return
	}

	if c.String("logfmt") == "none" {
		return
	}

	switch c.String("loglvl") {
	case "trace", "t":
		level = log.TraceLevel
	case "debug", "d":
		level = log.DebugLevel
	case "info", "i":
		level = log.InfoLevel
	case "warn", "warning", "w":
		level = log.WarnLevel
	case "error", "err", "e":
		level = log.ErrorLevel
	case "fatal", "f":
		level = log.FatalLevel
	default:
		level = log.InfoLevel
	}

	return
}

// WithFormat returns an option that configures a logger's format.
func WithFormat(c *cli.Context) log.Option {
	var fmt logrus.Formatter

	switch c.String("logfmt") {
	case "none":
	case "json":
		fmt = &logrus.JSONFormatter{PrettyPrint: c.Bool("prettyprint")}
	default:
		fmt = new(logrus.TextFormatter)
	}

	return log.WithFormatter(fmt)
}

func WithLogger(ctx context.Context, log log.Logger) context.Context {
	return context.WithValue(ctx, keyLogger{}, log)

}

func Logger(ctx context.Context) log.Logger {
	if l, ok := ctx.Value(keyLogger{}).(log.Logger); ok {
		return l
	}
	return nopLogger{}
}

func withErrWriter(c *cli.Context) log.Option {
	return log.WithWriter(c.App.ErrWriter)
}

type (
	keyLogger struct{}
)

type nopLogger struct{}

func (nopLogger) Fatal(...interface{})                     {}
func (nopLogger) Fatalf(string, ...interface{})            {}
func (nopLogger) Fatalln(...interface{})                   {}
func (nopLogger) Trace(...interface{})                     {}
func (nopLogger) Tracef(string, ...interface{})            {}
func (nopLogger) Traceln(...interface{})                   {}
func (nopLogger) Debug(...interface{})                     {}
func (nopLogger) Debugf(string, ...interface{})            {}
func (nopLogger) Debugln(...interface{})                   {}
func (nopLogger) Info(...interface{})                      {}
func (nopLogger) Infof(string, ...interface{})             {}
func (nopLogger) Infoln(...interface{})                    {}
func (nopLogger) Warn(...interface{})                      {}
func (nopLogger) Warnf(string, ...interface{})             {}
func (nopLogger) Warnln(...interface{})                    {}
func (nopLogger) Error(...interface{})                     {}
func (nopLogger) Errorf(string, ...interface{})            {}
func (nopLogger) Errorln(...interface{})                   {}
func (nopLogger) With(log.Loggable) log.Logger             { return nopLogger{} }
func (nopLogger) WithError(error) log.Logger               { return nopLogger{} }
func (nopLogger) WithField(string, interface{}) log.Logger { return nopLogger{} }
func (nopLogger) WithFields(logrus.Fields) log.Logger      { return nopLogger{} }
