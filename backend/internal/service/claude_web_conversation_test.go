package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func mustParseClaudeWebConversationRequest(t *testing.T, body string) *ParsedRequest {
	t.Helper()
	parsed, err := ParseGatewayRequest(NewRequestBodyRef([]byte(body)), PlatformAnthropic)
	require.NoError(t, err)
	return parsed
}

func TestPlanClaudeWebConversation_FirstTurnUsesFullPrompt(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	parsed := mustParseClaudeWebConversationRequest(t, `{
		"model":"claude-sonnet-5",
		"system":[{"type":"text","text":"be concise","cache_control":{"type":"ephemeral"}}],
		"messages":[{"role":"user","content":"first question"}]
	}`)

	plan, err := PlanClaudeWebConversation(parsed, "claude-sonnet-5", nil, now)

	require.NoError(t, err)
	require.False(t, plan.Reused)
	require.Equal(t, "first_turn", plan.MissReason)
	require.Contains(t, plan.Prompt, "[System]\nbe concise")
	require.Contains(t, plan.Prompt, "[Human]\nfirst question")
	require.Equal(t, 5*time.Minute, plan.TTL)
	require.NotEmpty(t, plan.DigestChain)
	require.Greater(t, plan.NewInputTokensEstimated, 0)
}

func TestPlanClaudeWebConversation_FollowUpUsesOnlyLatestUserTurn(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 1, 0, 0, time.UTC)
	first := mustParseClaudeWebConversationRequest(t, `{
		"model":"claude-sonnet-5",
		"system":"be concise",
		"messages":[{"role":"user","content":"first question"}]
	}`)
	second := mustParseClaudeWebConversationRequest(t, `{
		"model":"claude-sonnet-5",
		"system":"be concise",
		"messages":[
			{"role":"user","content":"first question"},
			{"role":"assistant","content":"first answer"},
			{"role":"user","content":"second question"}
		]
	}`)
	firstPlan, err := PlanClaudeWebConversation(first, "claude-sonnet-5", nil, now.Add(-time.Minute))
	require.NoError(t, err)
	state := &ClaudeWebConversationState{
		ConversationID:         "conv-1",
		ParentMessageUUID:      "assistant-turn-1",
		Model:                  "claude-sonnet-5",
		DigestChain:            firstPlan.DigestChain,
		ContextTokensEstimated: 1200,
		LastUsedAt:             now.Add(-time.Minute),
		TTLSeconds:             300,
	}

	plan, err := PlanClaudeWebConversation(second, "claude-sonnet-5", state, now)

	require.NoError(t, err)
	require.True(t, plan.Reused)
	require.Empty(t, plan.MissReason)
	require.Equal(t, "assistant-turn-1", plan.ParentMessageUUID)
	require.Equal(t, "second question", plan.Prompt)
	require.Equal(t, 1200, plan.ReusedInputTokensEstimated)
	require.Greater(t, plan.NewInputTokensEstimated, 0)
	require.Equal(t, 5*time.Minute, plan.TTL)
}

func TestPlanClaudeWebConversation_FollowUpNormalizesAnthropicTextBlocksAndCacheControl(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 1, 0, 0, time.UTC)
	first := mustParseClaudeWebConversationRequest(t, `{
		"model":"claude-sonnet-5",
		"system":[{"type":"text","text":"be concise","cache_control":{"type":"ephemeral","ttl":"5m"}}],
		"messages":[{"role":"user","content":[{"type":"text","text":"first question","cache_control":{"type":"ephemeral"}}]}]
	}`)
	firstPlan, err := PlanClaudeWebConversation(first, "claude-sonnet-5", nil, now.Add(-time.Minute))
	require.NoError(t, err)

	second := mustParseClaudeWebConversationRequest(t, `{
		"model":"claude-sonnet-5",
		"system":[{"type":"text","text":"be concise","cache_control":{"type":"ephemeral","ttl":"1h"}}],
		"messages":[
			{"role":"user","content":[{"type":"text","text":"first question"}]},
			{"role":"assistant","content":[{"type":"text","text":"first answer"}]},
			{"role":"user","content":[{"type":"text","text":"second question"}]}
		]
	}`)
	state := &ClaudeWebConversationState{
		ConversationID:         "conv-1",
		ParentMessageUUID:      "assistant-turn-1",
		Model:                  "claude-sonnet-5",
		DigestChain:            firstPlan.DigestChain + "-" + claudeWebAssistantDigest("first answer"),
		ContextTokensEstimated: 1200,
		LastUsedAt:             now.Add(-time.Minute),
		TTLSeconds:             300,
	}

	plan, err := PlanClaudeWebConversation(second, "claude-sonnet-5", state, now)

	require.NoError(t, err)
	require.True(t, plan.Reused)
	require.Empty(t, plan.MissReason)
	require.Equal(t, "second question", plan.Prompt)
}

func TestPlanClaudeWebConversation_DivergedHistoryRebuilds(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 1, 0, 0, time.UTC)
	first := mustParseClaudeWebConversationRequest(t, `{"messages":[{"role":"user","content":"first"}]}`)
	diverged := mustParseClaudeWebConversationRequest(t, `{
		"messages":[
			{"role":"user","content":"changed"},
			{"role":"assistant","content":"answer"},
			{"role":"user","content":"next"}
		]
	}`)
	state := &ClaudeWebConversationState{
		ConversationID:    "conv-1",
		ParentMessageUUID: "assistant-turn-1",
		Model:             "claude-sonnet-5",
		DigestChain:       BuildAnthropicDigestChain(first),
		LastUsedAt:        now.Add(-time.Minute),
		TTLSeconds:        300,
	}

	plan, err := PlanClaudeWebConversation(diverged, "claude-sonnet-5", state, now)

	require.NoError(t, err)
	require.False(t, plan.Reused)
	require.Equal(t, "history_diverged", plan.MissReason)
	require.Contains(t, plan.Prompt, "[Human]\nchanged")
}

func TestPlanClaudeWebConversation_ModelChangeRebuilds(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 1, 0, 0, time.UTC)
	parsed := mustParseClaudeWebConversationRequest(t, `{"messages":[{"role":"user","content":"first"}]}`)
	state := &ClaudeWebConversationState{
		ConversationID:    "conv-1",
		ParentMessageUUID: "assistant-turn-1",
		Model:             "claude-sonnet-5",
		DigestChain:       BuildAnthropicDigestChain(parsed),
		LastUsedAt:        now.Add(-time.Minute),
		TTLSeconds:        300,
	}

	plan, err := PlanClaudeWebConversation(parsed, "claude-opus-4-8", state, now)

	require.NoError(t, err)
	require.False(t, plan.Reused)
	require.Equal(t, "model_changed", plan.MissReason)
}

func TestClaudeWebConversationTTL_OneHour(t *testing.T) {
	parsed := mustParseClaudeWebConversationRequest(t, `{
		"cache_control":{"type":"ephemeral","ttl":"1h"},
		"messages":[{"role":"user","content":"hello"}]
	}`)

	require.Equal(t, time.Hour, ClaudeWebConversationTTL(parsed))
}

func TestPlanClaudeWebConversation_ExpiredStateRebuilds(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 10, 0, 0, time.UTC)
	parsed := mustParseClaudeWebConversationRequest(t, `{"messages":[{"role":"user","content":"first"}]}`)
	state := &ClaudeWebConversationState{
		ConversationID:    "conv-1",
		ParentMessageUUID: "assistant-turn-1",
		Model:             "claude-sonnet-5",
		DigestChain:       BuildAnthropicDigestChain(parsed),
		LastUsedAt:        now.Add(-6 * time.Minute),
		TTLSeconds:        300,
	}

	plan, err := PlanClaudeWebConversation(parsed, "claude-sonnet-5", state, now)

	require.NoError(t, err)
	require.False(t, plan.Reused)
	require.Equal(t, "ttl_expired", plan.MissReason)
}

func TestClaudeWebConversationUsage_RetainedFirstTurnCreatesFiveMinuteCache(t *testing.T) {
	plan := ClaudeWebConversationPlan{
		NewInputTokensEstimated: 10503,
		TTL:                     5 * time.Minute,
	}

	usage := ClaudeWebConversationUsage(plan, true)

	require.Zero(t, usage.InputTokens)
	require.Equal(t, 10503, usage.CacheCreationInputTokens)
	require.Equal(t, 10503, usage.CacheCreation5mTokens)
	require.Zero(t, usage.CacheCreation1hTokens)
	require.Zero(t, usage.CacheReadInputTokens)
}

func TestClaudeWebConversationUsage_RetainedFollowUpReportsCacheReadAndOneHourCreation(t *testing.T) {
	plan := ClaudeWebConversationPlan{
		Reused:                     true,
		ReusedInputTokensEstimated: 86918,
		NewInputTokensEstimated:    9230,
		TTL:                        time.Hour,
	}

	usage := ClaudeWebConversationUsage(plan, true)

	require.Zero(t, usage.InputTokens)
	require.Equal(t, 9230, usage.CacheCreationInputTokens)
	require.Zero(t, usage.CacheCreation5mTokens)
	require.Equal(t, 9230, usage.CacheCreation1hTokens)
	require.Equal(t, 86918, usage.CacheReadInputTokens)
}

func TestClaudeWebConversationUsage_FallbackKeepsOrdinaryInput(t *testing.T) {
	plan := ClaudeWebConversationPlan{
		ReusedInputTokensEstimated: 86918,
		NewInputTokensEstimated:    10503,
		TTL:                        5 * time.Minute,
	}

	usage := ClaudeWebConversationUsage(plan, false)

	require.Equal(t, 10503, usage.InputTokens)
	require.Zero(t, usage.CacheCreationInputTokens)
	require.Zero(t, usage.CacheReadInputTokens)
}
