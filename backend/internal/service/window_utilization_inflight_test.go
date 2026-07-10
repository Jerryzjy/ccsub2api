package service

import (
	"testing"
	"time"
)

// activeWindowAccount 构造一个"利用率模式已启用、窗口仍活跃"的账号。
func activeWindowAccount(util, perReqDelta float64) *Account {
	end := time.Now().Add(2 * time.Hour) // 窗口未到期，不触发"过期放行"短路
	extra := map[string]any{
		"window_utilization_limit":   0.8, // 80%
		"window_utilization_reserve": 0.1, // 10% → 红线 90%
		"session_window_utilization": util,
	}
	if perReqDelta > 0 {
		extra["passive_usage_util_per_req"] = perReqDelta
	}
	return &Account{
		Platform:         PlatformAnthropic,
		Type:             AccountTypeOAuth,
		Extra:            extra,
		SessionWindowEnd: &end,
	}
}

func TestUtilizationInFlightReserve_BlocksBeforeRedline(t *testing.T) {
	// 已记录利用率 82%（在 [80%,90%) 粘性区），但有 8 个在飞请求，每请求 ~1%。
	// 预扣 = 8 * 0.01 * 1.4 = 0.112 → 有效利用率 = 0.82 + 0.112 = 0.932 >= 0.9 → 完全不可调度。
	// 没有预扣时它会停在 StickyOnly（粘性放行）→ 高并发下继续冲过 100%，正是要修的。
	acc := activeWindowAccount(0.82, 0.01)

	if got := acc.CheckWindowCostSchedulability(0, 8); got != WindowCostNotSchedulable {
		t.Errorf("with 8 in-flight reserve, want NotSchedulable, got %d", got)
	}
	// 无在飞时（inFlight=0）保持原行为：82% 在粘性区。
	if got := acc.CheckWindowCostSchedulability(0, 0); got != WindowCostStickyOnly {
		t.Errorf("with 0 in-flight, want StickyOnly (unchanged behavior), got %d", got)
	}
}

func TestUtilizationInFlightReserve_LowUtilStillSchedulable(t *testing.T) {
	// 利用率很低（30%），即便有在飞请求也不该被误挡。
	acc := activeWindowAccount(0.30, 0.01)
	if got := acc.CheckWindowCostSchedulability(0, 5); got != WindowCostSchedulable {
		t.Errorf("low util must stay Schedulable, got %d", got)
	}
}

func TestUtilizationInFlightReserve_NoDeltaNoReserve(t *testing.T) {
	// 还没学到 per-req delta（早窗口）时，预扣为 0，退回原行为，不误挡。
	acc := activeWindowAccount(0.82, 0) // 无 delta
	if got := acc.CheckWindowCostSchedulability(0, 20); got != WindowCostStickyOnly {
		t.Errorf("without learned delta, behavior must be unchanged (StickyOnly), got %d", got)
	}
}

func TestUtilizationInFlightReserve_PushesGreenToSticky(t *testing.T) {
	// 78%（<80%，本应绿区放行）+ 足够在飞 → 预扣抬过 80% → 转粘性（提前收手），
	// 但还没到红线 90%，所以是 StickyOnly 而非 NotSchedulable。
	// 78% + 4*0.005*1.4=0.028 → 0.808，进入 [0.8,0.9) 粘性区。
	acc := activeWindowAccount(0.78, 0.005)
	if got := acc.CheckWindowCostSchedulability(0, 4); got != WindowCostStickyOnly {
		t.Errorf("in-flight reserve should push green->sticky, got %d", got)
	}
}
