//go:build unit

package handler

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEstimateAnthropicTokens_StringContent(t *testing.T) {
	// 4 chars of content -> 1 token (chars/4)
	body := []byte(`{"messages":[{"role":"user","content":"abcd"}]}`)
	require.Equal(t, int64(1), estimateAnthropicTokens(body))
}

func TestEstimateAnthropicTokens_BlockContent(t *testing.T) {
	// 8 text chars across two blocks -> 2 tokens; non-text block ignored.
	body := []byte(`{"messages":[{"role":"user","content":[` +
		`{"type":"text","text":"abcd"},` +
		`{"type":"image","source":{"data":"x"}},` +
		`{"type":"text","text":"efgh"}]}]}`)
	require.Equal(t, int64(2), estimateAnthropicTokens(body))
}

func TestEstimateAnthropicTokens_SystemString(t *testing.T) {
	// system 8 chars + message 4 chars = 12 chars -> 3 tokens
	body := []byte(`{"system":"sysprmpt","messages":[{"role":"user","content":"abcd"}]}`)
	require.Equal(t, int64(3), estimateAnthropicTokens(body))
}

func TestEstimateAnthropicTokens_SystemBlocks(t *testing.T) {
	// system blocks 4+4 chars + message 4 chars = 12 chars -> 3 tokens
	body := []byte(`{"system":[{"type":"text","text":"aaaa"},{"type":"text","text":"bbbb"}],` +
		`"messages":[{"role":"user","content":"cccc"}]}`)
	require.Equal(t, int64(3), estimateAnthropicTokens(body))
}

func TestEstimateAnthropicTokens_Chinese(t *testing.T) {
	// 8 Chinese runes counted as runes (not bytes) -> 8/4 = 2 tokens
	body := []byte(`{"messages":[{"role":"user","content":"中文测试内容超长"}]}`)
	require.Equal(t, int64(2), estimateAnthropicTokens(body))
}

func TestEstimateAnthropicTokens_ToolUseInput(t *testing.T) {
	// tool_use input raw JSON counted; raw is {"q":"abcd"} = 12 chars -> 3 tokens
	body := []byte(`{"messages":[{"role":"assistant","content":[` +
		`{"type":"tool_use","id":"t1","name":"search","input":{"q":"abcd"}}]}]}`)
	require.Equal(t, int64(3), estimateAnthropicTokens(body))
}

func TestEstimateAnthropicTokens_ToolResultString(t *testing.T) {
	// tool_result content raw JSON counted; raw is "abcdefgh" (with quotes) = 10 chars -> 2 tokens
	body := []byte(`{"messages":[{"role":"user","content":[` +
		`{"type":"tool_result","tool_use_id":"t1","content":"abcdefgh"}]}]}`)
	require.Equal(t, int64(2), estimateAnthropicTokens(body))
}

func TestEstimateAnthropicTokens_ToolResultBlocks(t *testing.T) {
	// tool_result content as []block: raw is [{"type":"text","text":"abcd"}] = 30 chars -> 7 tokens
	body := []byte(`{"messages":[{"role":"user","content":[` +
		`{"type":"tool_result","tool_use_id":"t1","content":[{"type":"text","text":"abcd"}]}]}]}`)
	require.Equal(t, int64(7), estimateAnthropicTokens(body))
}

func TestEstimateAnthropicTokens_Tools(t *testing.T) {
	// tools array raw JSON counted; raw is [{"name":"f"}] = 14 chars -> 3 tokens
	body := []byte(`{"messages":[{"role":"user","content":""}],"tools":[{"name":"f"}]}`)
	require.Equal(t, int64(3), estimateAnthropicTokens(body))
}

func TestEstimateAnthropicTokens_EmptyBody(t *testing.T) {
	require.Equal(t, int64(0), estimateAnthropicTokens([]byte{}))
}

func TestEstimateAnthropicTokens_NoMessages(t *testing.T) {
	require.Equal(t, int64(0), estimateAnthropicTokens([]byte(`{"model":"claude-x"}`)))
}
