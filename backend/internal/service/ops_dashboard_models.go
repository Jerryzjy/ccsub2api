package service

import "time"

type OpsDashboardFilter struct {
	StartTime time.Time
	EndTime   time.Time

	Platform string
	GroupID  *int64

	// QueryMode controls whether dashboard queries should use raw logs or pre-aggregated tables.
	// Expected values: auto/raw/preagg (see OpsQueryMode).
	QueryMode OpsQueryMode
}

type OpsRateSummary struct {
	Current float64 `json:"current"`
	Peak    float64 `json:"peak"`
	Avg     float64 `json:"avg"`
}

type OpsPercentiles struct {
	P50 *int `json:"p50_ms"`
	P90 *int `json:"p90_ms"`
	P95 *int `json:"p95_ms"`
	P99 *int `json:"p99_ms"`
	Avg *int `json:"avg_ms"`
	Max *int `json:"max_ms"`
}

type OpsDashboardOverview struct {
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Platform  string    `json:"platform"`
	GroupID   *int64    `json:"group_id"`

	// HealthScore is a backend-computed overall health score (0-100).
	// It is derived from the monitored metrics in this overview, plus best-effort system metrics/job heartbeats.
	HealthScore int `json:"health_score"`

	// Latest system-level snapshot (window=1m, global).
	SystemMetrics *OpsSystemMetricsSnapshot `json:"system_metrics"`

	// Background jobs health (heartbeats).
	JobHeartbeats []*OpsJobHeartbeat `json:"job_heartbeats"`

	SuccessCount         int64 `json:"success_count"`
	ErrorCountTotal      int64 `json:"error_count_total"`
	BusinessLimitedCount int64 `json:"business_limited_count"`

	ErrorCountSLA     int64 `json:"error_count_sla"`
	RequestCountTotal int64 `json:"request_count_total"`
	RequestCountSLA   int64 `json:"request_count_sla"`

	TokenConsumed int64 `json:"token_consumed"`

	SLA                          float64 `json:"sla"`
	ErrorRate                    float64 `json:"error_rate"`
	UpstreamErrorRate            float64 `json:"upstream_error_rate"`
	UpstreamErrorCountExcl429529 int64   `json:"upstream_error_count_excl_429_529"`
	Upstream429Count             int64   `json:"upstream_429_count"`
	Upstream529Count             int64   `json:"upstream_529_count"`

	QPS OpsRateSummary `json:"qps"`
	TPS OpsRateSummary `json:"tps"`

	Duration OpsPercentiles `json:"duration"`
	TTFT     OpsPercentiles `json:"ttft"`
}

type OpsLatencyHistogramBucket struct {
	Range string `json:"range"`
	Count int64  `json:"count"`
}

// OpsLatencyHistogramResponse is a coarse latency distribution histogram (success requests only).
// It is used by the Ops dashboard to quickly identify tail latency regressions.
type OpsLatencyHistogramResponse struct {
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Platform  string    `json:"platform"`
	GroupID   *int64    `json:"group_id"`

	TotalRequests int64                        `json:"total_requests"`
	Buckets       []*OpsLatencyHistogramBucket `json:"buckets"`
}

// OpsCacheClientType classifies a request's originating client for cache-hit-rate
// breakdown. Derived from usage_logs.user_agent at query time (not persisted).
type OpsCacheClientType string

const (
	OpsCacheClientClaudeCode OpsCacheClientType = "claude_code"
	OpsCacheClientThirdParty OpsCacheClientType = "third_party"
	OpsCacheClientUnknown    OpsCacheClientType = "unknown"
)

// OpsCacheHitRateRow is the cache-hit-rate for one (account, client_type) bucket.
//
// HitRate is cache_read / (input + cache_read + cache_creation); the denominator
// is the total cacheable input surface, so HitRate answers "what fraction of input
// tokens were served from cache". 0 when the denominator is 0.
type OpsCacheHitRateRow struct {
	AccountID  int64              `json:"account_id"`
	ClientType OpsCacheClientType `json:"client_type"`

	RequestCount        int64 `json:"request_count"`
	InputTokens         int64 `json:"input_tokens"`
	CacheReadTokens     int64 `json:"cache_read_tokens"`
	CacheCreationTokens int64 `json:"cache_creation_tokens"`

	HitRate float64 `json:"hit_rate"`
}

// OpsCacheHitRateReport is the cache-hit-rate breakdown for the dashboard card.
// Rows are per (account, client_type); ByClientType is the same data rolled up
// across accounts, which is what the headline card numbers use.
type OpsCacheHitRateReport struct {
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Platform  string    `json:"platform"`
	GroupID   *int64    `json:"group_id"`

	Rows         []*OpsCacheHitRateRow `json:"rows"`
	ByClientType []*OpsCacheHitRateRow `json:"by_client_type"`
}
