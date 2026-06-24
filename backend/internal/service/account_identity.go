package service

import "strings"

// AccountUpstreamIdentity 返回账号的上游身份键，用于去重与调度折叠。
//
// 优先级: credentials.account_uuid -> extra.account_uuid -> credentials.org_uuid
// -> credentials.email_address。返回空串表示无法识别该账号的上游身份
// （调用方应跳过去重/折叠，把该账号当作独立个体处理）。
//
// 这是防封策略的核心原语：同一上游 Anthropic 账号可能被重复导入成多条 account
// 记录，调度/冷却必须以"上游身份"而非 account.ID 为单位，否则同一会话会被劈裂到
// 多条记录上，破坏 prompt cache 命中率。
func AccountUpstreamIdentity(a *Account) string {
	if a == nil {
		return ""
	}
	if v := stringFromMap(a.Credentials, "account_uuid"); v != "" {
		return v
	}
	if v := stringFromMap(a.Extra, "account_uuid"); v != "" {
		return v
	}
	if v := stringFromMap(a.Credentials, "org_uuid"); v != "" {
		return v
	}
	if v := stringFromMap(a.Credentials, "email_address"); v != "" {
		return v
	}
	return ""
}

func stringFromMap(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if raw, ok := m[key]; ok {
		if s, ok := raw.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// foldByUpstreamIdentity 把同一上游身份的多个候选折叠为一个代表，
// 代表取负载率最低者，并列取账号 ID 最小者（保证确定性）。
// 无法识别身份的候选原样保留，互不折叠。
//
// 用于 Layer2 负载均衡前，避免在"同一上游账号的多条重复记录"之间做负载均衡/轮询
// （那会把同一会话甩到不同记录、丢失 prompt cache）。
func foldByUpstreamIdentity(accounts []accountWithLoad) []accountWithLoad {
	if len(accounts) <= 1 {
		return accounts
	}
	repByID := make(map[string]int, len(accounts)) // identity -> index in out
	out := make([]accountWithLoad, 0, len(accounts))
	for i := range accounts {
		id := AccountUpstreamIdentity(accounts[i].account)
		if id == "" {
			out = append(out, accounts[i])
			continue
		}
		if idx, ok := repByID[id]; ok {
			if betterRepresentative(accounts[i], out[idx]) {
				out[idx] = accounts[i]
			}
			continue
		}
		repByID[id] = len(out)
		out = append(out, accounts[i])
	}
	return out
}

func betterRepresentative(cand, cur accountWithLoad) bool {
	cl, ul := loadRateOf(cand), loadRateOf(cur)
	if cl != ul {
		return cl < ul
	}
	return cand.account.ID < cur.account.ID
}

func loadRateOf(a accountWithLoad) int {
	if a.loadInfo == nil {
		return 0
	}
	return a.loadInfo.LoadRate
}
