package service

import "testing"

func TestAccountUpstreamIdentity(t *testing.T) {
	cases := []struct {
		name string
		acc  *Account
		want string
	}{
		{"credentials uuid", &Account{Credentials: map[string]any{"account_uuid": "u-1"}}, "u-1"},
		{"extra uuid fallback", &Account{Extra: map[string]any{"account_uuid": "u-2"}}, "u-2"},
		{"org uuid fallback", &Account{Credentials: map[string]any{"org_uuid": "org-3"}}, "org-3"},
		{"email fallback", &Account{Credentials: map[string]any{"email_address": "a@b.com"}}, "a@b.com"},
		{"empty", &Account{}, ""},
		{"nil", nil, ""},
		{"prefer credentials over extra", &Account{Credentials: map[string]any{"account_uuid": "c"}, Extra: map[string]any{"account_uuid": "e"}}, "c"},
		{"trim whitespace", &Account{Credentials: map[string]any{"account_uuid": "  u-x  "}}, "u-x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := AccountUpstreamIdentity(tc.acc); got != tc.want {
				t.Fatalf("want %q got %q", tc.want, got)
			}
		})
	}
}

func TestFoldByUpstreamIdentity(t *testing.T) {
	mk := func(id int64, uuid string, load int) accountWithLoad {
		return accountWithLoad{
			account:  &Account{ID: id, Credentials: map[string]any{"account_uuid": uuid}},
			loadInfo: &AccountLoadInfo{AccountID: id, LoadRate: load},
		}
	}
	in := []accountWithLoad{
		mk(1, "A", 50), mk(41, "A", 10), // 同 UUID A，取负载低的 41
		mk(2, "B", 30),                  // 独立 B
		{account: &Account{ID: 9}, loadInfo: &AccountLoadInfo{AccountID: 9}}, // 无身份，保留
	}
	out := foldByUpstreamIdentity(in)
	ids := map[int64]bool{}
	for _, a := range out {
		ids[a.account.ID] = true
	}
	if len(out) != 3 {
		t.Fatalf("want 3 folded, got %d (ids=%v)", len(out), ids)
	}
	if !ids[41] || ids[1] {
		t.Fatalf("UUID A should fold to id=41 (lower load), got ids=%v", ids)
	}
	if !ids[2] || !ids[9] {
		t.Fatalf("B and no-identity must be kept, got ids=%v", ids)
	}
}

func TestFoldByUpstreamIdentity_TieBreakByID(t *testing.T) {
	mk := func(id int64, uuid string, load int) accountWithLoad {
		return accountWithLoad{
			account:  &Account{ID: id, Credentials: map[string]any{"account_uuid": uuid}},
			loadInfo: &AccountLoadInfo{AccountID: id, LoadRate: load},
		}
	}
	// 同 UUID 同负载，应取 ID 最小者（确定性）
	out := foldByUpstreamIdentity([]accountWithLoad{mk(41, "A", 10), mk(1, "A", 10)})
	if len(out) != 1 || out[0].account.ID != 1 {
		t.Fatalf("tie should keep lowest ID=1, got %+v", out)
	}
}
