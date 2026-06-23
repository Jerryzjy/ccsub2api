package service

import (
	"log/slog"
	"sync/atomic"
)

// 切号可观测性（诊断 prompt cache 命中率低）。
//
// prompt cache 按 upstream 账号（API key）隔离：同一会话若在多轮之间落到不同的
// 订阅账号，新账号看不到旧账号写入的缓存，必然整轮 miss。这是订阅号场景下命中率
// 低的头号嫌疑之一，但"切号频率"无法从代码静态判断，必须在运行时量化。
//
// 这里只做观测：在选号出口比对"上一轮该会话绑定的账号"与"本轮实际选中的账号"，
// 累加 sticky 命中 / 切号 / 无粘性 三类计数，并在切号时打一条 Info 日志。
// 不改变任何选号或转发行为。
var (
	// stickySwitchSameTotal 同一会话本轮仍命中上轮账号（缓存可复用）。
	stickySwitchSameTotal atomic.Int64
	// stickySwitchChangedTotal 同一会话本轮切到了不同账号（缓存按账号隔离 → 整轮 miss）。
	stickySwitchChangedTotal atomic.Int64
	// stickySwitchNoBindingTotal 本轮没有可比对的历史绑定（首轮、无 sessionHash、
	// 或绑定已过期）。这类无法判定是否切号，单列以免污染切号率分母。
	stickySwitchNoBindingTotal atomic.Int64
)

// GatewayStickySwitchStats 返回切号观测计数快照。
//
//	same       同会话仍命中上轮账号的次数
//	changed    同会话切到不同账号的次数（缓存 miss 来源）
//	noBinding  无历史绑定、无法判定的次数
//	switchRate changed / (same + changed)，无可判定样本时为 0
func GatewayStickySwitchStats() (same, changed, noBinding int64, switchRate float64) {
	same = stickySwitchSameTotal.Load()
	changed = stickySwitchChangedTotal.Load()
	noBinding = stickySwitchNoBindingTotal.Load()
	if denom := same + changed; denom > 0 {
		switchRate = float64(changed) / float64(denom)
	}
	return same, changed, noBinding, switchRate
}

// recordStickySwitch 比对上轮绑定账号与本轮选中账号，累加观测计数。
//
//	priorAccountID    本轮选号前从 cache/prefetch 读到的"上轮该会话账号"，0 表示无绑定
//	selectedAccountID 本轮最终选中的账号，<=0 视为本轮未成功选号（不计入）
//	sessionHash       会话指纹（仅用于日志，截断输出）
//	source            上轮绑定来源（cache / prefetch / ""），仅用于日志
//
// 该函数无副作用（只写原子计数 + 可能打一条日志），可安全放在选号出口的 defer 中。
func recordStickySwitch(priorAccountID, selectedAccountID int64, sessionHash, source string) {
	if selectedAccountID <= 0 {
		// 本轮没选出账号（错误/等待计划未落定），不计入切号统计。
		return
	}
	if priorAccountID <= 0 {
		stickySwitchNoBindingTotal.Add(1)
		return
	}
	if priorAccountID == selectedAccountID {
		stickySwitchSameTotal.Add(1)
		return
	}

	stickySwitchChangedTotal.Add(1)
	slog.Info("sticky.account_switched",
		"session_hash", shortSessionHash(sessionHash),
		"prior_account_id", priorAccountID,
		"selected_account_id", selectedAccountID,
		"sticky_source", source,
	)
}
