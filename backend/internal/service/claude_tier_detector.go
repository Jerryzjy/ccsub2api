package service

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// windowCostFn returns the standard dollar cost spent in the account's current
// 5h window. The bool is false when the cost cannot be determined, in which case
// tier detection is skipped for this observation.
type windowCostFn func(ctx context.Context, account *Account) (cost float64, ok bool)

// tierDetectMinUtilization is the utilization floor below which we refuse to
// infer a tier. budget = cost / utilization amplifies measurement noise as
// utilization approaches zero, so we wait until the window has accrued at least
// this much usage before dividing.
const tierDetectMinUtilization = 0.05

// detectAndStoreClaudeTier infers the Claude subscription tier (Pro / Max 5x /
// Max 20x) from the real dollars spent in the current 5h window and the
// utilization fraction Anthropic reports for that same window.
//
// The key identity is:
//
//	5h_budget = cost_spent / utilization_fraction
//
// Anthropic returns "anthropic-ratelimit-unified-5h-utilization" as a 0-1
// fraction, and we already track the standard cost spent in the window, so the
// budget — and therefore the tier — falls out directly with no per-request cost
// guessing. The tiers differ by roughly:
//
//	Pro:     ~$10 per 5h
//	Max 5x:  ~$50 per 5h
//	Max 20x: ~$100 per 5h
//
// We classify by the geometric midpoints between adjacent tiers so the boundary
// is symmetric in ratio space.
func detectAndStoreClaudeTier(ctx context.Context, account *Account, headers http.Header, accountRepo AccountRepository, costFn windowCostFn) {
	if account == nil || !account.IsAnthropicOAuthOrSetupToken() {
		return
	}

	// Skip if tier already set.
	if account.Extra != nil {
		if tier, ok := account.Extra["claude_tier"].(string); ok && tier != "" {
			return
		}
	}

	// Parse 5h utilization from response headers (0-1 fraction).
	utilStr := headers.Get("anthropic-ratelimit-unified-5h-utilization")
	if utilStr == "" {
		return
	}
	utilization, err := strconv.ParseFloat(utilStr, 64)
	if err != nil || utilization < tierDetectMinUtilization {
		return
	}

	if costFn == nil {
		return
	}
	cost, ok := costFn(ctx, account)
	if !ok || cost <= 0 {
		return
	}

	budget := cost / utilization
	tier := classifyClaudeTier(budget)

	logger.LegacyPrintf("service.ratelimit", "Claude tier detected: account=%d tier=%s budget=$%.2f cost=$%.2f util=%.4f",
		account.ID, tier, budget, cost, utilization)
	slog.Info("claude_tier_detected", "account_id", account.ID, "tier", tier, "budget", budget, "cost", cost, "utilization", utilization)

	if accountRepo != nil {
		_ = accountRepo.UpdateExtra(ctx, account.ID, map[string]any{
			"claude_tier": tier,
		})
	}
}

// classifyClaudeTier maps an inferred 5h dollar budget to a tier label using the
// geometric midpoints between the nominal tier budgets ($10 / $50 / $100):
//
//	< sqrt(10*50)  ≈ $22.4  -> Pro
//	< sqrt(50*100) ≈ $70.7  -> Max 5x
//	otherwise               -> Max 20x
func classifyClaudeTier(budget float64) string {
	const (
		proMax5xBoundary = 22.36 // sqrt(10 * 50)
		max5x20xBoundary = 70.71 // sqrt(50 * 100)
	)
	switch {
	case budget < proMax5xBoundary:
		return "Pro"
	case budget < max5x20xBoundary:
		return "Max_5x"
	default:
		return "Max_20x"
	}
}
