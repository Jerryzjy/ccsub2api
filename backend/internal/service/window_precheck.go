package service

import "time"

// ShouldSkipForWindowQuota 当账号窗口使用率 >= threshold 时返回 true（应提前停调度），
// 在被上游 429 之前主动退避，保护账号窗口额度。
//
// 读取 extra 中已有的 utilization 字段（passive_usage_7d_utilization /
// session_window_utilization），取最大者与阈值比较。无数据或阈值<=0 时返回 false。
//
// 5h 窗口（session_window_utilization）过期后，上游额度已自动重置，旧快照作废，
// 此处忽略它，避免账号停调度后无流量刷新、利用率永久停在高位被软封印。
// 与 CheckWindowCostSchedulability 的过期放行逻辑保持一致。
// 7d 利用率独立于 5h 窗口，不受此过期判断影响。
func ShouldSkipForWindowQuota(a *Account, threshold float64) bool {
	if a == nil || a.Extra == nil || threshold <= 0 {
		return false
	}
	fiveHourExpired := a.SessionWindowEnd != nil && !time.Now().Before(*a.SessionWindowEnd)
	maxUtil := -1.0
	for _, k := range []string{"passive_usage_7d_utilization", "session_window_utilization"} {
		if k == "session_window_utilization" && fiveHourExpired {
			continue // 5h 窗口已过期，旧快照作废
		}
		if v, ok := floatFromExtra(a.Extra, k); ok && v > maxUtil {
			maxUtil = v
		}
	}
	if maxUtil < 0 {
		return false
	}
	return maxUtil >= threshold
}

func floatFromExtra(m map[string]any, key string) (float64, bool) {
	raw, ok := m[key]
	if !ok {
		return 0, false
	}
	switch v := raw.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	}
	return 0, false
}
