//go:build unit

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// 构造一个超长 user content（约 N tokens = N*4 chars）。
func oversizedMessagesBody(t *testing.T, approxChars int) []byte {
	t.Helper()
	text := strings.Repeat("a", approxChars)
	payload := map[string]any{
		"model":      "claude-sonnet-4-5",
		"max_tokens": 256,
		"messages": []any{
			map[string]any{"role": "user", "content": text},
		},
	}
	b, err := json.Marshal(payload)
	require.NoError(t, err)
	return b
}

func newTooLongTestContext(t *testing.T, groupID int64, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	group := &service.Group{
		ID:       groupID,
		Hydrated: true,
		Platform: service.PlatformAnthropic,
		Status:   service.StatusActive,
	}
	req := httptest.NewRequest("POST", "/v1/messages", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), ctxkey.Group, group))
	c.Request = req

	apiKey := &service.APIKey{
		ID:      3001,
		UserID:  4001,
		GroupID: &groupID,
		Status:  service.StatusActive,
		User:    &service.User{ID: 4001, Concurrency: 10, Balance: 100},
		Group:   group,
	}
	c.Set(string(middleware.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: apiKey.UserID, Concurrency: 10})
	return c, rec
}

func TestGatewayHandlerMessages_TooLong_Blocked(t *testing.T) {
	gin.SetMode(gin.TestMode)

	group := &service.Group{ID: 5001, Hydrated: true, Platform: service.PlatformAnthropic, Status: service.StatusActive}
	h, cleanup := newTestGatewayHandler(t, group, nil)
	defer cleanup()
	h.cfg = &config.Config{}
	h.cfg.Gateway.MaxEstimatedTokens = 1000 // 阈值 1000 tokens

	// 4001 chars -> ~1000 tokens, > 1000 阈值
	body := oversizedMessagesBody(t, 4004)
	c, rec := newTooLongTestContext(t, group.ID, body)

	h.Messages(c)

	require.Equal(t, 413, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	errObj, ok := resp["error"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "invalid_request_error", errObj["type"])
	require.Contains(t, errObj["message"].(string), "请求内容过长")

	// 必须标记为 business_limited，使其排除出 SLA/错误率口径。
	require.True(t, service.HasOpsClientBusinessLimited(c))

	// 不能选中任何账号（拦截发生在账号选择之前）。
	_, selected := c.Get(opsAccountIDKey)
	require.False(t, selected, "no account should be selected when blocked")
}

func TestGatewayHandlerMessages_TooLong_DisabledWhenZero(t *testing.T) {
	gin.SetMode(gin.TestMode)

	group := &service.Group{ID: 5002, Hydrated: true, Platform: service.PlatformAnthropic, Status: service.StatusActive}
	h, cleanup := newTestGatewayHandler(t, group, nil)
	defer cleanup()
	h.cfg = &config.Config{}
	h.cfg.Gateway.MaxEstimatedTokens = 0 // 禁用

	body := oversizedMessagesBody(t, 4004)
	c, rec := newTooLongTestContext(t, group.ID, body)

	h.Messages(c)

	// 禁用时不应因超长被拦截（不会返回 413 + 中文提示）。
	require.NotEqual(t, 413, rec.Code)
	require.False(t, service.HasOpsClientBusinessLimited(c))
}
