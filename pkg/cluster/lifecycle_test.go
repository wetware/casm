package cluster

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLifecycle_Empty(t *testing.T) {
	t.Parallel()

	lx := lifecycle{}

	err := lx.Start()
	require.NoError(t, err)

	err = lx.Close()
	require.NoError(t, err)
}

func TestLifecycle_Succeed(t *testing.T) {
	t.Parallel()

	var h testHook
	lx := lifecycle{&h, &h, &h, &h, &h}

	err := lx.Start()
	require.NoError(t, err)

	assert.Equal(t, len(lx), int(h))

	err = lx.Close()
	require.NoError(t, err)

	assert.Zero(t, h)
}

func TestLifecycle_Abort(t *testing.T) {
	t.Parallel()

	var log []string

	lx := lifecycle{
		hook{
			OnStart: func() error {
				log = append(log, "h0 START")
				return nil
			},
			OnClose: func() error {
				log = append(log, "h0 STOP")
				return nil
			},
		},
		hook{
			OnStart: func() error {
				log = append(log, "h1 START")
				return nil
			},
			OnClose: func() error {
				log = append(log, "h1 STOP")
				return nil
			},
		},
		errHook{},
		panicHook{},
		panicHook{},
	}

	err := lx.Start()
	assert.EqualError(t, err, "test")

	assert.Equal(t,
		[]string{"h0 START", "h1 START", "h1 STOP", "h0 STOP"},
		log,
		"hooks should be closed in reverse order")

}

type testHook int

func (h *testHook) Start() error {
	*h++
	return nil
}

func (h *testHook) Close() error {
	*h--
	return nil
}

type errHook struct{}

func (errHook) Start() error { return fmt.Errorf("test") }

func (errHook) Close() error { return fmt.Errorf("errHook.Close was called") }

type panicHook struct{}

func (panicHook) Start() error { panic("start was calleds") }
func (panicHook) Close() error { panic("close was called") }
