//go:build unit

package handler

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// user 槽永远抢不到时，应在配置的 userWaitTimeout 内持续重试，超时后才返回 timeout 型 ConcurrencyError。
func TestConcurrencyHelper_UserWaitTimeout_Honored(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cache := &concurrencyCacheMock{
		acquireUserSlotFn: func(ctx context.Context, userID int64, maxConcurrency int, requestID string) (bool, error) {
			return false, nil // 永远抢不到
		},
	}
	// 自定义一个很短的 user 等待超时，便于快速断言。
	helper := NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, time.Second, 150*time.Millisecond)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/v1/messages", nil)

	streamStarted := false
	start := time.Now()
	release, err := helper.AcquireUserSlotWithWait(c, 101, 1, false, &streamStarted)
	elapsed := time.Since(start)

	require.Nil(t, release)
	require.Error(t, err)
	var ce *ConcurrencyError
	require.True(t, errors.As(err, &ce), "expected ConcurrencyError, got %T", err)
	require.True(t, ce.IsTimeout)
	// 必须等满了配置的超时才超时（留点裕量，至少 120ms）。
	require.GreaterOrEqual(t, elapsed, 120*time.Millisecond)
}

// 传 0 时回退到默认 maxConcurrencyWait（保持现有调用点行为不变）。
func TestConcurrencyHelper_UserWaitTimeout_ZeroFallsBackToDefault(t *testing.T) {
	cache := &concurrencyCacheMock{}
	helper := NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, time.Second, 0)
	require.Equal(t, maxConcurrencyWait, helper.userWaitTimeout)
}
