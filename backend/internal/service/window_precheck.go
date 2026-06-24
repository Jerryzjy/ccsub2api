package service

// ShouldSkipForWindowQuota 当账号窗口使用率 >= threshold 时返回 true（应提前停调度），
// 在被上游 429 之前主动退避，保护账号窗口额度。
//
// 读取 extra 中已有的 utilization 字段（passive_usage_7d_utilization /
// session_window_utilization），取最大者与阈值比较。无数据或阈值<=0 时返回 false。
func ShouldSkipForWindowQuota(a *Account, threshold float64) bool {
	if a == nil || a.Extra == nil || threshold <= 0 {
		return false
	}
	maxUtil := -1.0
	for _, k := range []string{"passive_usage_7d_utilization", "session_window_utilization"} {
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
