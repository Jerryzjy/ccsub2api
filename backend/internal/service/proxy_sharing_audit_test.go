package service

import "testing"

func TestDetectSharedProxyIPs(t *testing.T) {
	p := func(v int64) *int64 { return &v }
	accs := []*Account{
		{ID: 1, ProxyID: p(89), Credentials: map[string]any{"account_uuid": "A"}},
		{ID: 2, ProxyID: p(89), Credentials: map[string]any{"account_uuid": "B"}},
		{ID: 3, ProxyID: p(61), Credentials: map[string]any{"account_uuid": "C"}},
		// 同一上游账号 A 的重复记录共用 proxy 61，不应算风险（按上游身份去重）
		{ID: 4, ProxyID: p(61), Credentials: map[string]any{"account_uuid": "C"}},
		// 无代理 / 无身份，忽略
		{ID: 5, Credentials: map[string]any{"account_uuid": "D"}},
		{ID: 6, ProxyID: p(70)},
	}

	risks := DetectSharedProxyIPs(accs, 1)
	if len(risks) != 1 {
		t.Fatalf("want 1 shared-proxy risk (proxy 89 shared by A,B), got %d: %+v", len(risks), risks)
	}
	if risks[0].ProxyID != 89 || risks[0].UpstreamCount != 2 {
		t.Errorf("unexpected risk: %+v", risks[0])
	}

	// threshold=2: proxy 89 只有 2 个上游，不超过 2，无风险
	if r := DetectSharedProxyIPs(accs, 2); len(r) != 0 {
		t.Errorf("threshold=2 should report no risk, got %+v", r)
	}
}
