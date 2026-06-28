package service

import (
	"testing"
	"time"
)

func TestDeriveAccountQuotaSnapshot_StatusClassification(t *testing.T) {
	now := time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name          string
		acc           *Account
		currentInUse  int64
		wantAvailable bool
		wantRateLim   bool
		wantWindow    bool
		wantError     bool
		wantReauth    bool
	}{
		{
			name: "available idle account",
			acc: &Account{
				Status:      StatusActive,
				Schedulable: true,
				Concurrency: 5,
			},
			currentInUse:  0,
			wantAvailable: true,
		},
		{
			name: "rate limited account",
			acc: &Account{
				Status:           StatusActive,
				Schedulable:      true,
				Concurrency:      5,
				RateLimitResetAt: ptrTime(now.Add(30 * time.Minute)),
			},
			wantAvailable: false,
			wantRateLim:   true,
		},
		{
			name: "window full in utilization mode",
			acc: &Account{
				Status:      StatusActive,
				Schedulable: true,
				Concurrency: 5,
				Extra: map[string]any{
					"window_utilization_limit":   0.8,
					"session_window_utilization": 0.95,
				},
				SessionWindowEnd: ptrTime(now.Add(time.Hour)),
			},
			wantAvailable: true, // IsSchedulable() doesn't gate on window utilization; windowFull is tracked separately
			wantWindow:    true,
		},
		{
			name: "hard error -> reauth",
			acc: &Account{
				Status:       StatusError,
				Schedulable:  true,
				Concurrency:  5,
				ErrorMessage: "OAuth token refresh failed: unauthorized",
			},
			wantAvailable: false,
			wantError:     true,
			wantReauth:    true,
		},
		{
			name: "hard error -> generic",
			acc: &Account{
				Status:       StatusError,
				Schedulable:  true,
				Concurrency:  5,
				ErrorMessage: "upstream 500 internal error",
			},
			wantAvailable: false,
			wantError:     true,
			wantReauth:    false,
		},
		{
			name: "expired window zeroes utilization (not window-full)",
			acc: &Account{
				Status:      StatusActive,
				Schedulable: true,
				Concurrency: 5,
				Extra: map[string]any{
					"window_utilization_limit":   0.8,
					"session_window_utilization": 0.95,
				},
				SessionWindowEnd: ptrTime(now.Add(-time.Minute)), // expired
			},
			wantAvailable: true,
			wantWindow:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deriveAccountQuotaSnapshot(tt.acc, tt.currentInUse, now)
			if got.available != tt.wantAvailable {
				t.Errorf("available = %v, want %v", got.available, tt.wantAvailable)
			}
			if got.isRateLimited != tt.wantRateLim {
				t.Errorf("isRateLimited = %v, want %v", got.isRateLimited, tt.wantRateLim)
			}
			if got.windowFull != tt.wantWindow {
				t.Errorf("windowFull = %v, want %v", got.windowFull, tt.wantWindow)
			}
			if got.isError != tt.wantError {
				t.Errorf("isError = %v, want %v", got.isError, tt.wantError)
			}
			if got.isReauth != tt.wantReauth {
				t.Errorf("isReauth = %v, want %v", got.isReauth, tt.wantReauth)
			}
		})
	}
}

func TestAggregateAccountQuota_Counts(t *testing.T) {
	now := time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)
	snaps := []accountQuotaSnapshot{
		{available: true, currentInUse: 3, concurrency: 5},                  // in use
		{available: true, currentInUse: 0, concurrency: 5, utilization: 0},  // full idle
		{available: true, currentInUse: 0, concurrency: 5, utilization: 0.5}, // idle but not full
		{isRateLimited: true, concurrency: 5},                                // exhausted
		{windowFull: true, utilizationMode: true, utilizationLim: 0.8, utilization: 0.9, concurrency: 5}, // exhausted
		{isError: true, isReauth: true},                                     // reauth
		{isError: true},                                                     // generic error
	}

	r := aggregateAccountQuota(snaps, now)

	if r.TotalAccounts != 7 {
		t.Errorf("TotalAccounts = %d, want 7", r.TotalAccounts)
	}
	if r.AvailableAccounts != 3 {
		t.Errorf("AvailableAccounts = %d, want 3", r.AvailableAccounts)
	}
	if r.InUseAccounts != 1 {
		t.Errorf("InUseAccounts = %d, want 1", r.InUseAccounts)
	}
	if r.IdleAccounts != 2 {
		t.Errorf("IdleAccounts = %d, want 2", r.IdleAccounts)
	}
	if r.FullIdleAccounts != 1 {
		t.Errorf("FullIdleAccounts = %d, want 1", r.FullIdleAccounts)
	}
	if r.ExhaustedAccounts != 2 {
		t.Errorf("ExhaustedAccounts = %d, want 2", r.ExhaustedAccounts)
	}
	if r.ReauthAccounts != 1 {
		t.Errorf("ReauthAccounts = %d, want 1", r.ReauthAccounts)
	}
	if r.ErrorAccounts != 1 {
		t.Errorf("ErrorAccounts = %d, want 1", r.ErrorAccounts)
	}
}

func TestAggregateAccountQuota_RemainingCapacity(t *testing.T) {
	now := time.Now()

	t.Run("utilization mode averages headroom", func(t *testing.T) {
		snaps := []accountQuotaSnapshot{
			{available: true, utilizationMode: true, utilizationLim: 0.8, utilization: 0.4}, // remaining 0.5
			{available: true, utilizationMode: true, utilizationLim: 0.8, utilization: 0.8}, // remaining 0.0
		}
		r := aggregateAccountQuota(snaps, now)
		if got := r.RemainingCapacityRatio; got < 0.249 || got > 0.251 {
			t.Errorf("RemainingCapacityRatio = %v, want ~0.25", got)
		}
	})

	t.Run("fallback to available/total when no utilization mode", func(t *testing.T) {
		snaps := []accountQuotaSnapshot{
			{available: true},
			{available: true},
			{available: false},
			{available: false},
		}
		r := aggregateAccountQuota(snaps, now)
		if got := r.RemainingCapacityRatio; got < 0.49 || got > 0.51 {
			t.Errorf("RemainingCapacityRatio = %v, want ~0.5", got)
		}
	})
}

func TestAggregateAccountQuota_LoadAndSaturation(t *testing.T) {
	now := time.Now()

	t.Run("load and saturation computed over available accounts", func(t *testing.T) {
		snaps := []accountQuotaSnapshot{
			{available: true, currentInUse: 2, concurrency: 10},
			{available: true, currentInUse: 0, concurrency: 10},
		}
		r := aggregateAccountQuota(snaps, now)
		if got := r.PoolLoadRatio; got < 0.099 || got > 0.101 {
			t.Errorf("PoolLoadRatio = %v, want ~0.1", got)
		}
		if r.SaturationMultiple == nil || *r.SaturationMultiple < 9.99 || *r.SaturationMultiple > 10.01 {
			t.Errorf("SaturationMultiple = %v, want ~10", r.SaturationMultiple)
		}
	})

	t.Run("zero in-use yields nil saturation", func(t *testing.T) {
		snaps := []accountQuotaSnapshot{
			{available: true, currentInUse: 0, concurrency: 10},
		}
		r := aggregateAccountQuota(snaps, now)
		if r.PoolLoadRatio != 0 {
			t.Errorf("PoolLoadRatio = %v, want 0", r.PoolLoadRatio)
		}
		if r.SaturationMultiple != nil {
			t.Errorf("SaturationMultiple = %v, want nil", *r.SaturationMultiple)
		}
	})
}

func TestDeriveHealth(t *testing.T) {
	tests := []struct {
		name      string
		available int64
		total     int64
		load      float64
		want      string
	}{
		{"depleted when none available", 0, 10, 0.1, AccountPoolHealthDepleted},
		{"sustainable healthy", 5, 10, 0.5, AccountPoolHealthSustainable},
		{"tight when high load", 5, 10, 0.85, AccountPoolHealthTight},
		{"tight when low availability", 2, 10, 0.1, AccountPoolHealthTight},
		{"sustainable at exact floor", 3, 10, 0.79, AccountPoolHealthSustainable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deriveHealth(tt.available, tt.total, tt.load); got != tt.want {
				t.Errorf("deriveHealth(%d,%d,%v) = %q, want %q", tt.available, tt.total, tt.load, got, tt.want)
			}
		})
	}
}

func TestAggregateAccountQuota_RecoveryBuckets(t *testing.T) {
	now := time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)
	snaps := []accountQuotaSnapshot{
		// two accounts recovering in the same minute (~47m) -> merged
		{isRateLimited: true, concurrency: 10, recoverAt: ptrTime(now.Add(47 * time.Minute))},
		{windowFull: true, concurrency: 11, recoverAt: ptrTime(now.Add(47*time.Minute + 20*time.Second))},
		// one recovering at ~1h17m
		{isRateLimited: true, concurrency: 5, recoverAt: ptrTime(now.Add(77 * time.Minute))},
		// beyond the 4h window -> excluded
		{isRateLimited: true, concurrency: 5, recoverAt: ptrTime(now.Add(5 * time.Hour))},
		// already recovered (past) -> excluded
		{isRateLimited: true, concurrency: 5, recoverAt: ptrTime(now.Add(-time.Minute))},
	}

	r := aggregateAccountQuota(snaps, now)

	if len(r.RecoveryBuckets) != 2 {
		t.Fatalf("len(RecoveryBuckets) = %d, want 2", len(r.RecoveryBuckets))
	}
	first := r.RecoveryBuckets[0]
	if first.AfterSeconds != 47*60 {
		t.Errorf("first bucket AfterSeconds = %d, want %d", first.AfterSeconds, 47*60)
	}
	if first.AccountCount != 2 {
		t.Errorf("first bucket AccountCount = %d, want 2", first.AccountCount)
	}
	if first.RestoredConcurrency != 21 {
		t.Errorf("first bucket RestoredConcurrency = %d, want 21", first.RestoredConcurrency)
	}
	if r.RecoveryBuckets[1].AfterSeconds != 77*60 {
		t.Errorf("second bucket AfterSeconds = %d, want %d", r.RecoveryBuckets[1].AfterSeconds, 77*60)
	}
}

func TestMatchesReauth(t *testing.T) {
	tests := []struct {
		msg  string
		want bool
	}{
		{"", false},
		{"OAuth token expired", true},
		{"401 Unauthorized", true},
		{"failed to refresh credentials", true},
		{"upstream 500 internal error", false},
		{"connection reset by peer", false},
	}
	for _, tt := range tests {
		if got := matchesReauth(tt.msg); got != tt.want {
			t.Errorf("matchesReauth(%q) = %v, want %v", tt.msg, got, tt.want)
		}
	}
}
