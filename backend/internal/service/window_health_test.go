package service

import "testing"

func TestShouldSkipForWindowQuota(t *testing.T) {
	hi := &Account{Extra: map[string]any{"passive_usage_7d_utilization": 0.95}}
	lo := &Account{Extra: map[string]any{"passive_usage_7d_utilization": 0.5}}
	multi := &Account{Extra: map[string]any{
		"passive_usage_7d_utilization": 0.5,
		"session_window_utilization":   0.9, // 取最大者 0.9
	}}

	if !ShouldSkipForWindowQuota(hi, 0.8) {
		t.Error("0.95 >= 0.8 should skip")
	}
	if ShouldSkipForWindowQuota(lo, 0.8) {
		t.Error("0.5 < 0.8 should not skip")
	}
	if !ShouldSkipForWindowQuota(multi, 0.8) {
		t.Error("max(0.5,0.9)=0.9 >= 0.8 should skip")
	}
	if ShouldSkipForWindowQuota(&Account{}, 0.8) {
		t.Error("no util data should not skip")
	}
	if ShouldSkipForWindowQuota(hi, 0) {
		t.Error("threshold<=0 should never skip")
	}
}

func TestAccountHealthScore(t *testing.T) {
	healthy := AccountHealthInput{ErrorRate: 0.0, RecentRateLimits: 0, AvgLatencyMs: 800}
	sick := AccountHealthInput{ErrorRate: 0.6, RecentRateLimits: 5, AvgLatencyMs: 20000}

	hs := AccountHealthScore(healthy)
	ss := AccountHealthScore(sick)
	if hs <= ss {
		t.Errorf("healthy(%d) must score higher than sick(%d)", hs, ss)
	}
	if hs != 100 {
		t.Errorf("fully healthy should be 100, got %d", hs)
	}
	if ss < 0 || ss > 100 {
		t.Errorf("score out of range: %d", ss)
	}
}
