package service

import (
	"testing"
	"time"
)

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

// TestShouldSkipForWindowQuota_5hWindowExpired 验证 5h 窗口过期后，
// 旧的 session_window_utilization 快照作废，不应再据此停调度——
// 与 CheckWindowCostSchedulability 的过期放行逻辑保持一致，避免账号被软封印。
// 但 7d 利用率独立于 5h 窗口，过期判断不应影响它。
func TestShouldSkipForWindowQuota_5hWindowExpired(t *testing.T) {
	now := time.Now()
	past := now.Add(-time.Minute) // 5h 窗口已过期
	future := now.Add(time.Hour)  // 5h 窗口未过期

	// 5h 窗口过期 + 仅 5h 快照高(0.95)：应放行(不 skip)
	expired5h := &Account{
		SessionWindowEnd: &past,
		Extra: map[string]any{
			"session_window_utilization": 0.95,
		},
	}
	if ShouldSkipForWindowQuota(expired5h, 0.8) {
		t.Error("expired 5h window with stale util 0.95: should NOT skip")
	}

	// 5h 窗口未过期 + 5h 快照高(0.95)：仍应 skip
	active5h := &Account{
		SessionWindowEnd: &future,
		Extra: map[string]any{
			"session_window_utilization": 0.95,
		},
	}
	if !ShouldSkipForWindowQuota(active5h, 0.8) {
		t.Error("active 5h window with util 0.95: should skip")
	}

	// 5h 窗口过期，但 7d 利用率独立且高(0.95)：仍应 skip(7d 不受 5h 过期影响)
	expired5hButHigh7d := &Account{
		SessionWindowEnd: &past,
		Extra: map[string]any{
			"session_window_utilization":   0.95, // 5h 作废
			"passive_usage_7d_utilization": 0.95, // 7d 仍有效
		},
	}
	if !ShouldSkipForWindowQuota(expired5hButHigh7d, 0.8) {
		t.Error("expired 5h but high 7d util: should still skip on 7d")
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
