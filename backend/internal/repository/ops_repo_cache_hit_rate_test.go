package repository

import (
	"context"
	"time"

	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpsRepositoryGetCacheHitRateByClientType(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	filter := &service.OpsDashboardFilter{StartTime: start, EndTime: end}

	rows := sqlmock.NewRows([]string{
		"account_id", "client_type", "request_count",
		"input_tokens", "cache_read_tokens", "cache_creation_tokens",
	}).
		// account 1, claude_code: read 900 of (input 100 + read 900 + write 0) = 0.9
		AddRow(int64(1), "claude_code", int64(10), int64(100), int64(900), int64(0)).
		// account 1, third_party: read 0 of 1000 = 0
		AddRow(int64(1), "third_party", int64(5), int64(1000), int64(0), int64(0)).
		// account 2, unknown: all zero -> denom 0 -> 0 (no divide-by-zero)
		AddRow(int64(2), "unknown", int64(1), int64(0), int64(0), int64(0))

	mock.ExpectQuery(`FROM usage_logs ul`).
		WithArgs(start, end).
		WillReturnRows(rows)

	out, err := repo.GetCacheHitRateByClientType(context.Background(), filter)
	require.NoError(t, err)
	require.Len(t, out, 3)

	require.Equal(t, int64(1), out[0].AccountID)
	require.Equal(t, service.OpsCacheClientClaudeCode, out[0].ClientType)
	require.InDelta(t, 0.9, out[0].HitRate, 0.0001)

	require.Equal(t, service.OpsCacheClientThirdParty, out[1].ClientType)
	require.InDelta(t, 0.0, out[1].HitRate, 0.0001)

	// denom == 0 must not panic or NaN
	require.Equal(t, service.OpsCacheClientUnknown, out[2].ClientType)
	require.InDelta(t, 0.0, out[2].HitRate, 0.0001)

	require.NoError(t, mock.ExpectationsWereMet())
}

// The classification rule lives in one SQL fragment; this guards that the
// official-CC / third-party / unknown arms stay present so a careless edit to
// the CASE doesn't silently collapse a bucket.
func TestOpsCacheClientTypeCaseSQLArms(t *testing.T) {
	require.Contains(t, opsCacheClientTypeCaseSQL, "claude-cli/%")
	require.Contains(t, opsCacheClientTypeCaseSQL, "'claude_code'")
	require.Contains(t, opsCacheClientTypeCaseSQL, "'third_party'")
	require.Contains(t, opsCacheClientTypeCaseSQL, "'unknown'")
}
