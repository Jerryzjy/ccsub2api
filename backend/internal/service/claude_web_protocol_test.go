package service

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildClaudeWebPrompt(t *testing.T) {
	body := []byte(`{
		"system":[{"type":"text","text":"Follow instructions."}],
		"messages":[
			{"role":"user","content":[{"type":"text","text":"hello"}]},
			{"role":"assistant","content":"hi"},
			{"role":"user","content":"continue"}
		]
	}`)

	got, err := BuildClaudeWebPrompt(body)

	require.NoError(t, err)
	require.Equal(t, "[System]\nFollow instructions.\n\n[Human]\nhello\n\n[Assistant]\nhi\n\n[Human]\ncontinue\n\n[Assistant]\n", got)
}

func TestTranslateClaudeWebSSE(t *testing.T) {
	input := "event: content_block_delta\n" +
		"data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"hello\"}}\n\n" +
		"event: content_block_delta\n" +
		"data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\" world\"}}\n\n" +
		"event: message_stop\n" +
		"data: {\"type\":\"message_stop\"}\n\n"
	var output bytes.Buffer

	usage, err := TranslateClaudeWebSSE(
		strings.NewReader(input),
		&output,
		ClaudeWebStreamMeta{Model: "claude-sonnet-4-5", MessageID: "msg-test", InputTokens: 12},
	)

	require.NoError(t, err)
	require.Equal(t, 12, usage.InputTokens)
	require.Greater(t, usage.OutputTokens, 0)
	raw := output.String()
	require.Contains(t, raw, "event: message_start")
	require.Contains(t, raw, `"id":"msg-test"`)
	require.Contains(t, raw, "event: content_block_start")
	require.Contains(t, raw, `"text":"hello"`)
	require.Contains(t, raw, `"text":" world"`)
	require.Contains(t, raw, "event: content_block_stop")
	require.Contains(t, raw, `"stop_reason":"end_turn"`)
	require.Contains(t, raw, "event: message_stop")
}

func TestAggregateClaudeWebSSE(t *testing.T) {
	input := "event: content_block_delta\n" +
		"data: {\"delta\":{\"type\":\"text_delta\",\"text\":\"answer\"}}\n\n" +
		"event: message_stop\ndata: {}\n\n"

	body, usage, err := AggregateClaudeWebSSE(strings.NewReader(input), "claude-sonnet-4-5", "msg-1", 4)

	require.NoError(t, err)
	require.Equal(t, 4, usage.InputTokens)
	require.Greater(t, usage.OutputTokens, 0)
	var response struct {
		ID         string `json:"id"`
		Type       string `json:"type"`
		Role       string `json:"role"`
		Model      string `json:"model"`
		StopReason string `json:"stop_reason"`
		Content    []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	require.NoError(t, json.Unmarshal(body, &response))
	require.Equal(t, "msg-1", response.ID)
	require.Equal(t, "message", response.Type)
	require.Equal(t, "assistant", response.Role)
	require.Equal(t, "claude-sonnet-4-5", response.Model)
	require.Equal(t, "end_turn", response.StopReason)
	require.Equal(t, "answer", response.Content[0].Text)
}

func TestTranslateClaudeWebSSEReturnsSanitizedUpstreamError(t *testing.T) {
	input := "event: error\n" +
		"data: {\"error\":{\"message\":\"sessionKey=secret-value failed\"}}\n\n"

	_, err := TranslateClaudeWebSSE(
		strings.NewReader(input),
		io.Discard,
		ClaudeWebStreamMeta{Model: "claude-sonnet-4-5", MessageID: "msg-test"},
	)

	require.Error(t, err)
	require.NotContains(t, err.Error(), "secret-value")
	require.Contains(t, err.Error(), "Claude Web session expired")
}

func TestBuildClaudeWebPromptRejectsUnsupportedBlocks(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":[{"type":"image","source":{"type":"base64"}}]}]}`)

	_, err := BuildClaudeWebPrompt(body)

	require.ErrorIs(t, err, ErrClaudeWebUnsupportedContent)
}

func TestBuildClaudeWebCompletion(t *testing.T) {
	body := []byte(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"hello"}]}`)

	got, err := BuildClaudeWebCompletion(body, "claude-sonnet-4-5")

	require.NoError(t, err)
	require.Equal(t, "claude-sonnet-4-5", got.Model)
	require.Equal(t, "[Human]\nhello\n\n[Assistant]\n", got.Prompt)
	require.NotEmpty(t, got.ParentMessageUUID)
	require.NotEmpty(t, got.TurnMessageUUIDs.HumanMessageUUID)
	require.NotEmpty(t, got.TurnMessageUUIDs.AssistantMessageUUID)
	require.Equal(t, "Asia/Shanghai", got.Timezone)
	require.Equal(t, "en-US", got.Locale)
	require.Equal(t, "messages", got.RenderingMode)
	require.Empty(t, got.Attachments)
	require.Empty(t, got.Files)
	require.Empty(t, got.SyncSources)

	raw, err := json.Marshal(got)
	require.NoError(t, err)
	require.NotContains(t, string(raw), `"tools"`)
}

func TestResolveClaudeWebModel(t *testing.T) {
	tests := map[string]string{
		"claude-sonnet-4-5-20250929": "claude-sonnet-5",
		"claude-3-7-sonnet-latest":   "claude-sonnet-5",
		"claude-opus-4-20250514":     "claude-opus-4-8",
		"claude-haiku-4-5-20251001":  "claude-haiku-4-5",
		"claude-sonnet-4-6":          "claude-sonnet-4-6",
	}
	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			require.Equal(t, want, ResolveClaudeWebModel(input))
		})
	}
}
