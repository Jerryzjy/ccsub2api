package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestStripAssistantPrefillIfUnsupported(t *testing.T) {
	t.Run("non-empty prefill is removed for unsupported model", func(t *testing.T) {
		body := []byte(`{"model":"claude-opus-4-8","messages":[` +
			`{"role":"user","content":"hi"},` +
			`{"role":"assistant","content":"根据分析,"}` +
			`]}`)

		out, err := stripAssistantPrefillIfUnsupported(body, "claude-opus-4-8")
		require.NoError(t, err)

		msgs := gjson.GetBytes(out, "messages").Array()
		require.Len(t, msgs, 1)
		require.Equal(t, "user", msgs[len(msgs)-1].Get("role").String())
	})

	t.Run("empty prefill is removed for unsupported model", func(t *testing.T) {
		body := []byte(`{"model":"claude-opus-4-8","messages":[` +
			`{"role":"user","content":"hi"},` +
			`{"role":"assistant","content":""}` +
			`]}`)

		out, err := stripAssistantPrefillIfUnsupported(body, "claude-opus-4-8")
		require.NoError(t, err)

		msgs := gjson.GetBytes(out, "messages").Array()
		require.Len(t, msgs, 1)
		require.Equal(t, "user", msgs[len(msgs)-1].Get("role").String())
	})

	t.Run("array-content prefill is removed", func(t *testing.T) {
		body := []byte(`{"model":"claude-opus-4-8","messages":[` +
			`{"role":"user","content":"hi"},` +
			`{"role":"assistant","content":[{"type":"text","text":"continue from here"}]}` +
			`]}`)

		out, err := stripAssistantPrefillIfUnsupported(body, "claude-opus-4-8")
		require.NoError(t, err)

		msgs := gjson.GetBytes(out, "messages").Array()
		require.Len(t, msgs, 1)
		require.Equal(t, "user", msgs[len(msgs)-1].Get("role").String())
	})

	t.Run("multiple trailing assistant messages are all removed", func(t *testing.T) {
		body := []byte(`{"model":"claude-opus-4-8","messages":[` +
			`{"role":"user","content":"hi"},` +
			`{"role":"assistant","content":"a"},` +
			`{"role":"assistant","content":"b"}` +
			`]}`)

		out, err := stripAssistantPrefillIfUnsupported(body, "claude-opus-4-8")
		require.NoError(t, err)

		msgs := gjson.GetBytes(out, "messages").Array()
		require.Len(t, msgs, 1)
		require.Equal(t, "user", msgs[len(msgs)-1].Get("role").String())
	})

	t.Run("conversation ending with user is left untouched", func(t *testing.T) {
		body := []byte(`{"model":"claude-opus-4-8","messages":[` +
			`{"role":"user","content":"hi"},` +
			`{"role":"assistant","content":"hello"},` +
			`{"role":"user","content":"bye"}` +
			`]}`)

		out, err := stripAssistantPrefillIfUnsupported(body, "claude-opus-4-8")
		require.NoError(t, err)
		require.Equal(t, string(body), string(out))
	})

	t.Run("supported model passes prefill through unchanged", func(t *testing.T) {
		body := []byte(`{"model":"claude-sonnet-4-5","messages":[` +
			`{"role":"user","content":"hi"},` +
			`{"role":"assistant","content":"根据分析,"}` +
			`]}`)

		out, err := stripAssistantPrefillIfUnsupported(body, "claude-sonnet-4-5")
		require.NoError(t, err)
		require.Equal(t, string(body), string(out))
	})

	t.Run("all-assistant messages collapse to empty", func(t *testing.T) {
		body := []byte(`{"model":"claude-opus-4-8","messages":[` +
			`{"role":"assistant","content":"a"},` +
			`{"role":"assistant","content":"b"}` +
			`]}`)

		out, err := stripAssistantPrefillIfUnsupported(body, "claude-opus-4-8")
		require.NoError(t, err)
		require.Len(t, gjson.GetBytes(out, "messages").Array(), 0)
	})

	t.Run("empty messages array is a no-op", func(t *testing.T) {
		body := []byte(`{"model":"claude-opus-4-8","messages":[]}`)

		out, err := stripAssistantPrefillIfUnsupported(body, "claude-opus-4-8")
		require.NoError(t, err)
		require.Equal(t, string(body), string(out))
	})
}
