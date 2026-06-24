package service

// SharedProxyRisk 描述一个被多个上游账号共用的出口代理。
// 多个不同上游 Anthropic 账号共用同一出口 IP 是典型的封号关联风险来源。
type SharedProxyRisk struct {
	ProxyID       int64
	UpstreamCount int
	UpstreamIDs   []string
}

// DetectSharedProxyIPs 统计每个 proxy 被多少个不同上游身份使用，
// 超过 threshold 的列为风险（threshold=1 表示任何被 >1 个上游共用的代理都告警）。
//
// 仅统计能识别上游身份（account_uuid 等）且绑定了代理的账号；
// 同一上游账号的多条重复记录共用同一代理不算风险（按上游身份去重计数）。
func DetectSharedProxyIPs(accounts []*Account, threshold int) []SharedProxyRisk {
	byProxy := make(map[int64]map[string]struct{})
	for _, a := range accounts {
		if a == nil || a.ProxyID == nil {
			continue
		}
		id := AccountUpstreamIdentity(a)
		if id == "" {
			continue
		}
		if byProxy[*a.ProxyID] == nil {
			byProxy[*a.ProxyID] = make(map[string]struct{})
		}
		byProxy[*a.ProxyID][id] = struct{}{}
	}
	var risks []SharedProxyRisk
	for pid, ids := range byProxy {
		if len(ids) > threshold {
			uids := make([]string, 0, len(ids))
			for u := range ids {
				uids = append(uids, u)
			}
			risks = append(risks, SharedProxyRisk{ProxyID: pid, UpstreamCount: len(ids), UpstreamIDs: uids})
		}
	}
	return risks
}
