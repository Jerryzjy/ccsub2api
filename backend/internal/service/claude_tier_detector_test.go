package service

import (
	"context"
	"net/http"
	"testing"
)

func headerWith5hUtil(util string) http.Header {
	h := http.Header{}
	h.Set("anthropic-ratelimit-unified-5h-utilization", util)
	return h
}

func TestDetectClaudeTier_InfersTierFromBudget(t *testing.T) {
	tests := []struct {
		name     string
		cost     float64
		util     string
		wantTier string
	}{
		// budget = cost / util
		{"pro: $5 at 50% -> $10 budget", 5.0, "0.50", "Pro"},
		{"pro: $2 at 20% -> $10 budget", 2.0, "0.20", "Pro"},
		{"max5x: $25 at 50% -> $50 budget", 25.0, "0.50", "Max_5x"},
		{"max5x: $10 at 20% -> $50 budget", 10.0, "0.20", "Max_5x"},
		{"max20x: $50 at 50% -> $100 budget", 50.0, "0.50", "Max_20x"},
		{"max20x: $20 at 20% -> $100 budget", 20.0, "0.20", "Max_20x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &sessionWindowMockRepo{}
			acc := &Account{ID: 1, Platform: "anthropic", Type: "oauth"}
			costFn := func(context.Context, *Account) (float64, bool) { return tt.cost, true }

			detectAndStoreClaudeTier(context.Background(), acc, headerWith5hUtil(tt.util), repo, costFn)

			if len(repo.updateExtraCalls) != 1 {
				t.Fatalf("expected 1 UpdateExtra call, got %d", len(repo.updateExtraCalls))
			}
			got := repo.updateExtraCalls[0].Updates["claude_tier"]
			if got != tt.wantTier {
				t.Fatalf("claude_tier = %v, want %v", got, tt.wantTier)
			}
		})
	}
}

func TestDetectClaudeTier_SkipsWhenTierAlreadySet(t *testing.T) {
	repo := &sessionWindowMockRepo{}
	acc := &Account{ID: 1, Platform: "anthropic", Type: "oauth", Extra: map[string]any{"claude_tier": "Max_5x"}}
	costFn := func(context.Context, *Account) (float64, bool) { return 25.0, true }

	detectAndStoreClaudeTier(context.Background(), acc, headerWith5hUtil("0.50"), repo, costFn)

	if len(repo.updateExtraCalls) != 0 {
		t.Fatalf("expected no UpdateExtra calls when tier already set, got %d", len(repo.updateExtraCalls))
	}
}

func TestDetectClaudeTier_SkipsWhenUtilizationTooLow(t *testing.T) {
	// Utilization below the reliability floor: dividing by a tiny number is noisy,
	// so detection should wait for more usage instead of guessing.
	repo := &sessionWindowMockRepo{}
	acc := &Account{ID: 1, Platform: "anthropic", Type: "oauth"}
	costFn := func(context.Context, *Account) (float64, bool) { return 0.1, true }

	detectAndStoreClaudeTier(context.Background(), acc, headerWith5hUtil("0.01"), repo, costFn)

	if len(repo.updateExtraCalls) != 0 {
		t.Fatalf("expected no UpdateExtra calls when utilization too low, got %d", len(repo.updateExtraCalls))
	}
}

func TestDetectClaudeTier_SkipsWhenCostUnavailable(t *testing.T) {
	repo := &sessionWindowMockRepo{}
	acc := &Account{ID: 1, Platform: "anthropic", Type: "oauth"}
	costFn := func(context.Context, *Account) (float64, bool) { return 0, false }

	detectAndStoreClaudeTier(context.Background(), acc, headerWith5hUtil("0.50"), repo, costFn)

	if len(repo.updateExtraCalls) != 0 {
		t.Fatalf("expected no UpdateExtra calls when cost unavailable, got %d", len(repo.updateExtraCalls))
	}
}

func TestDetectClaudeTier_SkipsNonAnthropicOAuth(t *testing.T) {
	repo := &sessionWindowMockRepo{}
	acc := &Account{ID: 1, Platform: "openai", Type: "oauth"}
	costFn := func(context.Context, *Account) (float64, bool) { return 25.0, true }

	detectAndStoreClaudeTier(context.Background(), acc, headerWith5hUtil("0.50"), repo, costFn)

	if len(repo.updateExtraCalls) != 0 {
		t.Fatalf("expected no UpdateExtra calls for non-anthropic-oauth, got %d", len(repo.updateExtraCalls))
	}
}
