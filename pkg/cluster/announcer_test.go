package cluster

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wetware/casm/pkg/cluster/pulse"
)

func TestEmitter(t *testing.T) {
	t.Parallel()

	e := newEmitter()
	e.SetTTL(pulse.DefaultTTL)

	t.Run("NewTicker", func(t *testing.T) {
		ticker := e.NewTicker()
		defer ticker.Stop()

		assert.Equal(t, pulse.DefaultTTL/2, ticker.Interval,
			"ticker interval should be set to half of TTL")
	})

	t.Run("NewBackoff", func(t *testing.T) {
		backoff := e.NewBackoff()
		assert.Equal(t, 2., backoff.Factor,
			"should have factor of 2")
		assert.Equal(t, pulse.DefaultTTL/10, backoff.Min,
			"minimum backoff time should be 10%% of TTL")
		assert.Equal(t, time.Minute*15, backoff.Max,
			"maximum backoff time should be 15 minutes")
		assert.True(t, backoff.Jitter,
			"backoff time should be jittered")
	})

	t.Run("Emit", func(t *testing.T) {
		e.Preparer = preparerFunc(func(meta pulse.Setter) error {
			return meta.SetMeta(map[string]string{
				"foo": "bar",
			})
		})

		err := e.Emit(context.Background(), publisherFunc(func(ctx context.Context, b []byte, po ...pubsub.PubOpt) error {
			p, err := e.Message().MarshalPacked()
			if err == nil {
				assert.Equal(t, p, b)
			}

			return err
		}))
		require.NoError(t, err, "should succeed")

		t.Run("CheckHostname", func(t *testing.T) {
			hostname, _ := os.Hostname()
			name, err := e.Host()
			require.NoError(t, err, "should return hostname")
			require.Equal(t, hostname, name,
				"hostname should match host")
		})

		t.Run("CheckMeta", func(t *testing.T) {
			meta, err := e.Meta()
			require.NoError(t, err)

			assert.Equal(t, 1, meta.Len(),
				"metadata should be set")

			value, err := meta.Get("foo")
			require.NoError(t, err)
			require.Equal(t, "bar", value)
		})
	})

	t.Run("EmitFail", func(t *testing.T) {
		testErr := errors.New("test")

		e.Preparer = preparerFunc(func(meta pulse.Setter) error {
			return testErr
		})

		err := e.Emit(context.Background(), publisherFunc(nil))
		assert.ErrorIs(t, err, testErr, "errors should match")
	})
}

type preparerFunc func(pulse.Setter) error

func (prepare preparerFunc) Prepare(meta pulse.Setter) error {
	return prepare(meta)
}

type publisherFunc func(context.Context, []byte, ...pubsub.PubOpt) error

func (publish publisherFunc) Publish(ctx context.Context, b []byte, opt ...pubsub.PubOpt) error {
	return publish(ctx, b, opt...)
}
