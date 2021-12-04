package cluster

import (
	"go.uber.org/multierr"
)

type hookable interface {
	Start() error
	Close() error
}

type lifecycle []hookable

func (lx lifecycle) Start() (err error) {
	for i, h := range lx {
		if err = h.Start(); err == nil {
			continue
		}

		_ = lx[:i].Close()
		break
	}

	return
}

func (lx lifecycle) Close() error {
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

type hook struct {
	OnStart func() error
	OnClose func() error
}

func (h hook) Start() (err error) {
	if h.OnStart != nil {
		err = h.OnStart()
	}
	return
}

func (h hook) Close() (err error) {
	if h.OnClose != nil {
		err = h.OnClose()
	}
	return
}
