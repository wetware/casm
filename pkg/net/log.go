package net

import (
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/lthibault/log"
	"github.com/sirupsen/logrus"
)

// Logger is a wrapper around github.com/lthibault/log with
// convenience methods for logging connections and streams.
type Logger struct{ log.Logger }

func (l Logger) With(v log.Loggable) Logger               { return Logger{l.Logger.With(v)} }
func (l Logger) WithError(err error) Logger               { return Logger{l.Logger.WithError(err)} }
func (l Logger) WithField(s string, v interface{}) Logger { return Logger{l.Logger.WithField(s, v)} }
func (l Logger) WithFields(log logrus.Fields) Logger      { return Logger{l.Logger.WithFields(log)} }

func (l Logger) WithConn(c network.Conn) Logger {
	return l.With(log.F{
		"peer": c.RemotePeer(),
		"conn": c.ID(),
	})
}

func (l Logger) WithStream(s network.Stream) Logger {
	return l.WithConn(s.Conn()).With(log.F{
		"stream": s.ID(),
		"proto":  s.Protocol(),
	})
}

func (l Logger) ReportError(err error) {
	l.WithError(err).Debug("error in rpc connection")
}
