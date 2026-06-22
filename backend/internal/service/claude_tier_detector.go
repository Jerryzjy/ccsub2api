package service

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// detectClaudeTierFromHeaders inspects anthropic-ratelimit response headers
// to infer the Claude subscription tier (Pro / Max 5x / Max 20x).
//
// Detection method: on the FIRST successful request for an account that has
// no tier set yet, record the utilization. The utilization increase from a
// single small request reveals the total budget:
//   - Pro ($20):   ~$10 per 5h → small request causes ~0.5-2% utilization jump
//   - Max 5x ($100): ~$50 per 5h → same request causes ~0.1-0.4% jump
//   - Max 20x ($200): ~$100 per 5h → same request causes ~0.05-0.2% jump
//
// Simpler heuristic: check the 7d window utilization after a known cost.
// But since we don't know exact cost at header-read time, we use a
// practical shortcut: if the account has processed requests and the
// utilization is very low despite significant usage, it's a higher tier.
//
// Actual implementation: we detect from response headers at the gateway level
// and store the tier in account.extra.claude_tier.
func detectAndStoreClaudeTier(ctx context.Context, account *Account, headers http.Header, accountRepo AccountRepository) {
	if account == nil || !account.IsAnthropicOAuthOrSetupToken() {
		return
	}

	// Skip if tier already set
	if account.Extra != nil {
		if tier, ok := account.Extra["claude_tier"].(string); ok && tier != "" {
			return
		}
	}

	// Parse 5h utilization from response headers
	utilStr := headers.Get("anthropic-ratelimit-unified-5h-utilization")
	if utilStr == "" {
		return
	}
	utilization, err := strconv.ParseFloat(utilStr, 64)
	if err != nil || utilization <= 0 {
		return
	}

	// Get the cost of this request from usage (we don't have it here directly,
	// so we use a different approach: store initial utilization on first request,
	// then on second request compare the delta to infer tier)
	//
	// For now, use a simpler heuristic based on the 5h status header behavior:
	// Claude returns "anthropic-ratelimit-unified-5h-status: allowed" for all tiers,
	// but the utilization growth rate differs. We store the utilization on first
	// observation and check on subsequent calls.

	// Check if we've stored a previous utilization sample
	var prevUtil float64
	if account.Extra != nil {
		if v, ok := account.Extra["_tier_detect_util"].(float64); ok {
			prevUtil = v
		}
	}

	if prevUtil == 0 {
		// First observation: store utilization for future comparison
		if accountRepo != nil {
			_ = accountRepo.UpdateExtra(ctx, account.ID, map[string]any{
				"_tier_detect_util": utilization,
			})
		}
		slog.Info("claude_tier_detect_first_sample", "account_id", account.ID, "utilization", utilization)
		return
	}

	// Second observation: calculate delta
	delta := utilization - prevUtil
	if delta <= 0 {
		// Window may have reset; re-record
		if accountRepo != nil {
			_ = accountRepo.UpdateExtra(ctx, account.ID, map[string]any{
				"_tier_detect_util": utilization,
			})
		}
		return
	}

	// Infer tier from utilization delta per request.
	// A single Claude Code request typically costs $0.01-$0.10.
	// Pro:   $10 budget → delta ≈ 0.001-0.01 (0.1%-1%)
	// 5x:    $50 budget → delta ≈ 0.0002-0.002 (0.02%-0.2%)
	// 20x:  $100 budget → delta ≈ 0.0001-0.001 (0.01%-0.1%)
	//
	// Use thresholds:
	//   delta >= 0.003 (0.3%) → Pro
	//   delta >= 0.0005 (0.05%) → Max 5x
	//   delta < 0.0005 → Max 20x
	var tier string
	switch {
	case delta >= 0.003:
		tier = "Pro"
	case delta >= 0.0005:
		tier = "Max_5x"
	default:
		tier = "Max_20x"
	}

	logger.LegacyPrintf("service.ratelimit", "Claude tier detected: account=%d tier=%s delta=%.6f prev=%.6f curr=%.6f",
		account.ID, tier, delta, prevUtil, utilization)

	// Store tier and clean up detection data
	if accountRepo != nil {
		_ = accountRepo.UpdateExtra(ctx, account.ID, map[string]any{
			"claude_tier":       tier,
			"_tier_detect_util": nil, // cleanup
		})
	}
}
