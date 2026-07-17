package service

import (
	"context"
	"sort"
	"strings"
	"time"
)

// Pool health bands.
const (
	AccountPoolHealthSustainable = "sustainable" // 可持续
	AccountPoolHealthTight       = "tight"       // 紧张
	AccountPoolHealthDepleted    = "depleted"    // 枯竭
)

const (
	// accountQuotaRecoveryWindow bounds the recovery forecast to the next 4h.
	accountQuotaRecoveryWindow = 4 * time.Hour
	// fullIdleUtilizationCeil is the upper bound on utilization for an account to
	// count as "满血闲置" (full-blood idle): essentially untouched in its 5h window.
	fullIdleUtilizationCeil = 0.05
	// healthAvailableRatioFloor / healthLoadCeil define the "可持续" band.
	healthAvailableRatioFloor = 0.3
	healthLoadCeil            = 0.8
)

// reauthKeywords flag an errored account as needing re-authorization rather than
// a generic error. Matched case-insensitively against the account error message.
var reauthKeywords = []string{"token", "auth", "unauthorized", "401", "refresh"}

// OpsAccountQuotaRecoveryBucket is one minute-bucket of accounts recovering quota.
type OpsAccountQuotaRecoveryBucket struct {
	AfterSeconds        int64 `json:"after_seconds"`
	AccountCount        int64 `json:"account_count"`
	RestoredConcurrency int64 `json:"restored_concurrency"`
}

// OpsAccountQuotaReport is the aggregated Claude account-pool quota snapshot
// rendered by the ops dashboard "可用额度监控" card.
type OpsAccountQuotaReport struct {
	Platform    string    `json:"platform"`
	GroupID     *int64    `json:"group_id"`
	GeneratedAt time.Time `json:"generated_at"`

	TotalAccounts     int64 `json:"total_accounts"`
	AvailableAccounts int64 `json:"available_accounts"`
	InUseAccounts     int64 `json:"in_use_accounts"`
	IdleAccounts      int64 `json:"idle_accounts"`
	ExhaustedAccounts int64 `json:"exhausted_accounts"`
	FullIdleAccounts  int64 `json:"full_idle_accounts"`
	ReauthAccounts    int64 `json:"reauth_accounts"`
	ErrorAccounts     int64 `json:"error_accounts"`

	RemainingCapacityRatio float64  `json:"remaining_capacity_ratio"`
	PoolLoadRatio          float64  `json:"pool_load_ratio"`
	SaturationMultiple     *float64 `json:"saturation_multiple"`
	Health                 string   `json:"health"`
	DepletionETASeconds    *int64   `json:"depletion_eta_seconds"`

	RecoveryBuckets []OpsAccountQuotaRecoveryBucket `json:"recovery_buckets"`
}

// accountQuotaSnapshot is the per-account derived view feeding the aggregation.
// Kept separate from the DB Account so the aggregation is pure and testable.
type accountQuotaSnapshot struct {
	concurrency     int64
	currentInUse    int64
	utilizationMode bool
	utilization     float64 // 0..1, zeroed when the 5h window has expired
	utilizationLim  float64 // 0..1

	available     bool
	isRateLimited bool
	windowFull    bool
	isError       bool
	isReauth      bool

	recoverAt *time.Time
}

// deriveAccountQuotaSnapshot projects a single account onto the quota view.
func deriveAccountQuotaSnapshot(acc *Account, currentInUse int64, now time.Time) accountQuotaSnapshot {
	snap := accountQuotaSnapshot{
		concurrency:  int64(acc.Concurrency),
		currentInUse: currentInUse,
	}

	limit := acc.getExtraFloat64("window_utilization_limit")
	snap.utilizationMode = limit > 0
	snap.utilizationLim = limit

	windowExpired := acc.SessionWindowEnd != nil && !now.Before(*acc.SessionWindowEnd)
	if !windowExpired {
		snap.utilization = acc.getExtraFloat64("session_window_utilization")
	}

	snap.isError = acc.Status == StatusError
	if snap.isError {
		snap.isReauth = matchesReauth(acc.ErrorMessage)
	} else {
		snap.isRateLimited = acc.RateLimitResetAt != nil && now.Before(*acc.RateLimitResetAt)
		snap.windowFull = snap.utilizationMode && snap.utilization >= snap.utilizationLim
	}

	snap.available = isAccountAvailableAt(acc, now)

	// Recovery time only applies to "已打满" accounts (rate-limited or window-full),
	// not hard errors which need manual intervention.
	if snap.isRateLimited && acc.RateLimitResetAt != nil {
		snap.recoverAt = acc.RateLimitResetAt
	}
	if snap.windowFull && acc.SessionWindowEnd != nil {
		if snap.recoverAt == nil || acc.SessionWindowEnd.Before(*snap.recoverAt) {
			snap.recoverAt = acc.SessionWindowEnd
		}
	}

	return snap
}

func matchesReauth(msg string) bool {
	if msg == "" {
		return false
	}
	lower := strings.ToLower(msg)
	for _, kw := range reauthKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// isAccountAvailableAt mirrors Account.IsSchedulable() but evaluates time-based
// gates against the supplied now, so the ops snapshot is consistent and testable.
func isAccountAvailableAt(acc *Account, now time.Time) bool {
	if acc.Status != StatusActive || !acc.Schedulable {
		return false
	}
	if acc.AutoPauseOnExpired && acc.ExpiresAt != nil && !now.Before(*acc.ExpiresAt) {
		return false
	}
	if acc.OverloadUntil != nil && now.Before(*acc.OverloadUntil) {
		return false
	}
	if acc.RateLimitResetAt != nil && now.Before(*acc.RateLimitResetAt) {
		return false
	}
	if acc.TempUnschedulableUntil != nil && now.Before(*acc.TempUnschedulableUntil) {
		return false
	}
	if acc.SupportsLocalQuotaControl() && acc.IsQuotaExceeded() {
		return false
	}
	return true
}

// aggregateAccountQuota folds per-account snapshots into the report metrics.
// Pure function: all wall-clock dependence is passed in via now.
func aggregateAccountQuota(snaps []accountQuotaSnapshot, now time.Time) OpsAccountQuotaReport {
	report := OpsAccountQuotaReport{
		RecoveryBuckets: []OpsAccountQuotaRecoveryBucket{},
	}

	var (
		availableCapacity int64
		availableInUse    int64
		utilSum           float64
		utilCount         int64
	)

	recovers := make([]recoverItem, 0)

	for _, s := range snaps {
		report.TotalAccounts++

		if s.available {
			report.AvailableAccounts++
			if s.currentInUse > 0 {
				report.InUseAccounts++
			} else {
				report.IdleAccounts++
				if s.utilization <= fullIdleUtilizationCeil {
					report.FullIdleAccounts++
				}
			}
			availableCapacity += s.concurrency
			availableInUse += s.currentInUse
		}

		if s.isRateLimited || s.windowFull {
			report.ExhaustedAccounts++
		}
		if s.isError {
			if s.isReauth {
				report.ReauthAccounts++
			} else {
				report.ErrorAccounts++
			}
		}

		if s.utilizationMode && s.utilizationLim > 0 {
			remaining := (s.utilizationLim - s.utilization) / s.utilizationLim
			utilSum += clamp01(remaining)
			utilCount++
		}

		if s.recoverAt != nil {
			delta := s.recoverAt.Sub(now)
			if delta > 0 && delta <= accountQuotaRecoveryWindow {
				recovers = append(recovers, recoverItem{
					afterSeconds: int64(delta.Seconds()),
					concurrency:  s.concurrency,
				})
			}
		}
	}

	// Remaining capacity: average headroom of utilization-mode accounts; fall back
	// to available/total when no account runs in utilization mode.
	if utilCount > 0 {
		report.RemainingCapacityRatio = utilSum / float64(utilCount)
	} else if report.TotalAccounts > 0 {
		report.RemainingCapacityRatio = float64(report.AvailableAccounts) / float64(report.TotalAccounts)
	}

	if availableCapacity > 0 {
		report.PoolLoadRatio = float64(availableInUse) / float64(availableCapacity)
	}
	if availableInUse > 0 {
		mult := float64(availableCapacity) / float64(availableInUse)
		report.SaturationMultiple = &mult
	}

	report.Health = deriveHealth(report.AvailableAccounts, report.TotalAccounts, report.PoolLoadRatio)
	report.RecoveryBuckets = bucketRecoveries(recovers)

	return report
}

func deriveHealth(available, total int64, loadRatio float64) string {
	if available == 0 {
		return AccountPoolHealthDepleted
	}
	availableRatio := 0.0
	if total > 0 {
		availableRatio = float64(available) / float64(total)
	}
	if availableRatio >= healthAvailableRatioFloor && loadRatio < healthLoadCeil {
		return AccountPoolHealthSustainable
	}
	return AccountPoolHealthTight
}

// recoverItem is one account's recovery timing, used to build minute buckets.
type recoverItem struct {
	afterSeconds int64
	concurrency  int64
}

// bucketRecoveries groups recovering accounts by whole-minute, ascending by time.
func bucketRecoveries(items []recoverItem) []OpsAccountQuotaRecoveryBucket {
	if len(items) == 0 {
		return []OpsAccountQuotaRecoveryBucket{}
	}

	byMinute := make(map[int64]*OpsAccountQuotaRecoveryBucket)
	for _, it := range items {
		minute := it.afterSeconds / 60
		b, ok := byMinute[minute]
		if !ok {
			b = &OpsAccountQuotaRecoveryBucket{AfterSeconds: minute * 60}
			byMinute[minute] = b
		}
		b.AccountCount++
		b.RestoredConcurrency += it.concurrency
	}

	out := make([]OpsAccountQuotaRecoveryBucket, 0, len(byMinute))
	for _, b := range byMinute {
		out = append(out, *b)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].AfterSeconds < out[j].AfterSeconds
	})
	return out
}

// GetAccountQuotaMonitor returns the aggregated Claude account-pool quota snapshot.
//
// Scope is limited to anthropic accounts; platform/group filters match the dashboard.
func (s *OpsService) GetAccountQuotaMonitor(ctx context.Context, platformFilter string, groupIDFilter *int64) (*OpsAccountQuotaReport, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}

	// This card is Claude-only for now; force the platform regardless of input.
	platformFilter = PlatformAnthropic

	accounts, err := s.listAllAccountsForOps(ctx, platformFilter)
	if err != nil {
		return nil, err
	}

	if groupIDFilter != nil && *groupIDFilter > 0 {
		filtered := make([]Account, 0, len(accounts))
		for _, acc := range accounts {
			for _, grp := range acc.Groups {
				if grp != nil && grp.ID == *groupIDFilter {
					filtered = append(filtered, acc)
					break
				}
			}
		}
		accounts = filtered
	}

	now := time.Now()
	loadMap := s.getAccountsLoadMapBestEffort(ctx, accounts)

	snaps := make([]accountQuotaSnapshot, 0, len(accounts))
	for i := range accounts {
		acc := &accounts[i]
		if acc.ID <= 0 {
			continue
		}
		currentInUse := int64(0)
		if load := loadMap[acc.ID]; load != nil {
			currentInUse = int64(load.CurrentConcurrency)
		}
		snaps = append(snaps, deriveAccountQuotaSnapshot(acc, currentInUse, now))
	}

	report := aggregateAccountQuota(snaps, now)
	report.Platform = platformFilter
	report.GroupID = groupIDFilter
	report.GeneratedAt = now

	return &report, nil
}
