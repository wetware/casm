// Package service provides licecycle management.
package service

import (
	"go.uber.org/multierr"
)

type Service interface {
	Start() error
	Close() error
}

type Set []Service

func (lx Set) Start() (err error) {
	for i, h := range lx {
		if err = h.Start(); err == nil {
			continue
		}

		_ = lx[:i].Close()
		break
	}

	return
}

func (lx Set) Close() error {
	for i, j := 0, len(lx)-1; i < j; i, j = i+1, j-1 {
		lx[i], lx[j] = lx[j], lx[i]
	}

	var es []error
	for _, h := range lx {
		if err := h.Close(); err != nil {
			es = append(es, err)
		}
	}

	return multierr.Combine(es...)
}

type Hook struct {
	OnStart func() error
	OnClose func() error
}

func (h Hook) Start() (err error) {
	if h.OnStart != nil {
		err = h.OnStart()
	}
	return
}

func (h Hook) Close() (err error) {
	if h.OnClose != nil {
		err = h.OnClose()
	}
	return
}
