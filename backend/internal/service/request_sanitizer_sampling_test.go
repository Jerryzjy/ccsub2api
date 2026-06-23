package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestStripSamplingParamsIfModelRejects_RemovesForRejectingModel(t *testing.T) {
	body := []byte(`{"model":"claude-opus-4-7","temperature":1,"top_p":0.9,"top_k":40,"max_tokens":100}`)
	got := stripSamplingParamsIfModelRejects(body, "claude-opus-4-7")
	require.False(t, gjson.GetBytes(got, "temperature").Exists())
	require.False(t, gjson.GetBytes(got, "top_p").Exists())
	require.False(t, gjson.GetBytes(got, "top_k").Exists())
	// unrelated fields preserved
	require.Equal(t, int64(100), gjson.GetBytes(got, "max_tokens").Int())
}

func TestStripSamplingParamsIfModelRejects_NoOpForAcceptingModel(t *testing.T) {
	body := []byte(`{"model":"claude-sonnet-4-5","temperature":1,"top_p":0.9}`)
	got := stripSamplingParamsIfModelRejects(body, "claude-sonnet-4-5")
	require.True(t, gjson.GetBytes(got, "temperature").Exists())
	require.True(t, gjson.GetBytes(got, "top_p").Exists())
}

// sanitizeRequestParams is the converged entry point; confirm a rejecting model
// gets temperature stripped through it (the path the passthrough forward uses).
func TestSanitizeRequestParams_StripsSamplingForRejectingModel(t *testing.T) {
	body := []byte(`{"model":"claude-opus-4-7","temperature":1,"max_tokens":100}`)
	got := sanitizeRequestParams(body, "claude-opus-4-7")
	require.False(t, gjson.GetBytes(got, "temperature").Exists())
}
