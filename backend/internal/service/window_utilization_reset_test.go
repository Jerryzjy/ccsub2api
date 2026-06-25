package service

import (
	"testing"
	"time"
)

// TestUtilizationSchedulability_WindowExpiredResets 验证 utilization 比例模式下，
// 当上游 5h 窗口已过期（SessionWindowEnd 到点）时，旧的利用率快照作废，
// 账号应重新进入调度——上游窗口重置后自动恢复，无需后台探测。
func TestUtilizationSchedulability_WindowExpiredResets(t *testing.T) {
	now := time.Now()
	past := now.Add(-time.Minute)   // 已过期
	future := now.Add(time.Hour)    // 未过期
	windowStart := now.Add(-5 * time.Hour)

	// 窗口已过期 + 利用率停在 0.95：上游已重置，应可调度。
	expired := &Account{
		SessionWindowStart: &windowStart,
		SessionWindowEnd:   &past,
		Extra: map[string]any{
			"window_utilization_limit":   0.8,
			"session_window_utilization": 0.95,
		},
	}
	if got := expired.CheckWindowCostSchedulability(0); got != WindowCostSchedulable {
		t.Errorf("expired window with stale util 0.95: want Schedulable, got %v", got)
	}

	// 窗口未过期 + 利用率 0.95：仍应被拦截（不可调度）。
	active := &Account{
		SessionWindowStart: &windowStart,
		SessionWindowEnd:   &future,
		Extra: map[string]any{
			"window_utilization_limit":   0.8,
			"session_window_utilization": 0.95,
		},
	}
	if got := active.CheckWindowCostSchedulability(0); got != WindowCostNotSchedulable {
		t.Errorf("active window util 0.95: want NotSchedulable, got %v", got)
	}

	// 窗口从未初始化（End=nil）+ 利用率 0.95：不能误判为过期放行，仍按比例拦截。
	noWindow := &Account{
		Extra: map[string]any{
			"window_utilization_limit":   0.8,
			"session_window_utilization": 0.95,
		},
	}
	if got := noWindow.CheckWindowCostSchedulability(0); got != WindowCostNotSchedulable {
		t.Errorf("nil window util 0.95: want NotSchedulable, got %v", got)
	}
}
