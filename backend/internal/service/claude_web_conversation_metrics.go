package service

import "sync/atomic"

type ClaudeWebConversationMetricsSnapshot struct {
	EligibleTotal     int64   `json:"eligible_total"`
	HitTotal          int64   `json:"hit_total"`
	UnsupportedTotal  int64   `json:"unsupported_total"`
	RebuildTotal      int64   `json:"rebuild_total"`
	CacheUnavailable  int64   `json:"cache_unavailable_total"`
	EligibleReuseRate float64 `json:"eligible_reuse_rate"`
}

var claudeWebConversationMetrics struct {
	eligible         atomic.Int64
	hit              atomic.Int64
	unsupported      atomic.Int64
	rebuild          atomic.Int64
	cacheUnavailable atomic.Int64
}

func SnapshotClaudeWebConversationMetrics() ClaudeWebConversationMetricsSnapshot {
	eligible := claudeWebConversationMetrics.eligible.Load()
	hit := claudeWebConversationMetrics.hit.Load()
	rate := 0.0
	if eligible > 0 {
		rate = float64(hit) / float64(eligible)
	}
	return ClaudeWebConversationMetricsSnapshot{
		EligibleTotal:     eligible,
		HitTotal:          hit,
		UnsupportedTotal:  claudeWebConversationMetrics.unsupported.Load(),
		RebuildTotal:      claudeWebConversationMetrics.rebuild.Load(),
		CacheUnavailable:  claudeWebConversationMetrics.cacheUnavailable.Load(),
		EligibleReuseRate: rate,
	}
}

func recordClaudeWebConversationPlan(plan ClaudeWebConversationPlan) {
	if plan.Reused {
		claudeWebConversationMetrics.eligible.Add(1)
		claudeWebConversationMetrics.hit.Add(1)
		return
	}
	if plan.MissReason == "unsupported_follow_up" {
		claudeWebConversationMetrics.eligible.Add(1)
		claudeWebConversationMetrics.unsupported.Add(1)
	}
}
