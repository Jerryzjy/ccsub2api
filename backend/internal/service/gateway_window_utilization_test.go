package service

import (
	"context"
	"testing"
)

// TestIsAccountSchedulableForWindowCost_UtilizationMode 验证 utilization 比例模式
// （window_utilization_limit，例如 80%）在调度时被真正执行。
//
// 回归场景：用户在前端选择「利用率%」模式后，extra 只有 window_utilization_limit，
// 没有 window_cost_limit。此前 isAccountSchedulableForWindowCost 因为
// GetWindowCostLimit()==0 而提前 return true，导致比例上限完全失效。
func TestIsAccountSchedulableForWindowCost_UtilizationMode(t *testing.T) {
	svc := &GatewayService{}

	// 利用率 95% >= 限制 80% + 预留 10% = 90%，应完全不可调度。
	over := &Account{
		ID:       1,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"window_utilization_limit":   0.8,
			"session_window_utilization": 0.95,
		},
	}
	if svc.isAccountSchedulableForWindowCost(context.Background(), over, false) {
		t.Error("util 0.95 >= limit+reserve 0.90: account must NOT be schedulable")
	}

	// 利用率 85%：>= 限制 80% 但 < 90%，仅粘性会话可调度。
	stickyZone := &Account{
		ID:       2,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"window_utilization_limit":   0.8,
			"session_window_utilization": 0.85,
		},
	}
	if svc.isAccountSchedulableForWindowCost(context.Background(), stickyZone, false) {
		t.Error("util 0.85 in sticky zone: non-sticky request must NOT be schedulable")
	}
	if !svc.isAccountSchedulableForWindowCost(context.Background(), stickyZone, true) {
		t.Error("util 0.85 in sticky zone: sticky request SHOULD be schedulable")
	}

	// 利用率 50% < 限制 80%：正常可调度。
	under := &Account{
		ID:       3,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"window_utilization_limit":   0.8,
			"session_window_utilization": 0.5,
		},
	}
	if !svc.isAccountSchedulableForWindowCost(context.Background(), under, false) {
		t.Error("util 0.5 < limit 0.8: account SHOULD be schedulable")
	}
}
