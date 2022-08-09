package pulse_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	api "github.com/wetware/casm/internal/api/pulse"
	"github.com/wetware/casm/pkg/cluster/pulse"
)

func TestHeartbeat_MarshalUnmarshal(t *testing.T) {
	t.Parallel()

	var h pulse.Heartbeat
	require.NotPanics(t, func() {
		h = pulse.NewHeartbeat()
	}, "heartbeat uses single-segment arena and should never panic")

	api.Heartbeat(h).SetTtl(42)
	require.Equal(t, time.Millisecond*42, h.TTL())

	err := api.Heartbeat(h).SetHostname("test.node.local")
	require.NoError(t, err, "should set hostname")

	fields, err := api.Heartbeat(h).NewMeta(3)
	require.NoError(t, err, "should allocate meta fields")

	for i, entry := range []struct{ key, value string }{
		{key: "cloud_region", value: "us-east-1"},
		{key: "special_field", value: "special-value"},
		{key: "home", value: "/home/wetware"},
	} {
		require.NoError(t, fields.At(i).SetKey(entry.key),
			"must set key=%s", entry.key)
		require.NoError(t, fields.At(i).SetValue(entry.value),
			"must set value=%s", entry.value)
	}

	// marshal
	b, err := h.MarshalBinary()
	require.NoError(t, err)
	require.NotEmpty(t, b)

	t.Logf("payload size:  %d bytes", len(b))

	// unmarshal
	hb2 := pulse.Heartbeat{}
	err = hb2.UnmarshalBinary(b)
	require.NoError(t, err)

	assert.Equal(t, h.TTL(), hb2.TTL())

	meta, err := h.Meta()
	require.NoError(t, err, "should return meta")

	for key, want := range map[string]string{
		"cloud_region":  "us-east-1",
		"special_field": "special-value",
		"home":          "/home/wetware",
		"missing":       "",
	} {
		value, err := meta.Get(key)
		require.NoError(t, err)
		require.Equal(t, want, value)
	}
}
