package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRollupCacheHitRateByClientType_WeightsByTokens(t *testing.T) {
	rows := []*OpsCacheHitRateRow{
		// Two claude_code accounts with very different volumes. Rollup must
		// recompute from SUMMED tokens, not average the per-account rates.
		//   acct 1 rate = 990/1000 = 0.99   (large volume)
		//   acct 2 rate = 0/100   = 0.00     (tiny volume)
		//   averaging the rates would give 0.495
		//   weighted = 990 / (1000 + 100) = 0.9 -> this is what we assert
		{AccountID: 1, ClientType: OpsCacheClientClaudeCode, InputTokens: 10, CacheReadTokens: 990, CacheCreationTokens: 0},
		{AccountID: 2, ClientType: OpsCacheClientClaudeCode, InputTokens: 100, CacheReadTokens: 0, CacheCreationTokens: 0},
		{AccountID: 3, ClientType: OpsCacheClientThirdParty, InputTokens: 200, CacheReadTokens: 300, CacheCreationTokens: 0},
	}

	out := rollupCacheHitRateByClientType(rows)
	require.Len(t, out, 2)

	// claude_code first (rollup preserves CC -> third_party -> unknown order)
	require.Equal(t, OpsCacheClientClaudeCode, out[0].ClientType)
	require.Equal(t, int64(990), out[0].CacheReadTokens)
	require.Equal(t, int64(110), out[0].InputTokens)
	// weighted 990 / (110 + 990 + 0) = 0.9, distinctly NOT the 0.495 average
	require.InDelta(t, 0.9, out[0].HitRate, 0.0001)

	require.Equal(t, OpsCacheClientThirdParty, out[1].ClientType)
	require.InDelta(t, 300.0/500.0, out[1].HitRate, 0.0001)
}

func TestRollupCacheHitRateByClientType_EmptyAndZeroDenom(t *testing.T) {
	require.Empty(t, rollupCacheHitRateByClientType(nil))

	out := rollupCacheHitRateByClientType([]*OpsCacheHitRateRow{
		{ClientType: OpsCacheClientUnknown, InputTokens: 0, CacheReadTokens: 0, CacheCreationTokens: 0},
	})
	require.Len(t, out, 1)
	require.InDelta(t, 0.0, out[0].HitRate, 0.0001)
}
