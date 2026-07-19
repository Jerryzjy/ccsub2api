//go:build unit

package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/oauth"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type cookieAuthClaudeClient struct {
	sessionKey   string
	scope        string
	isSetupToken bool
}

func (c *cookieAuthClaudeClient) GetOrganizationUUID(_ context.Context, sessionKey, _ string) (string, error) {
	c.sessionKey = sessionKey
	return "org-1", nil
}

func (c *cookieAuthClaudeClient) GetAuthorizationCode(_ context.Context, _ string, _ string, scope, _ string, _ string, _ string) (string, error) {
	c.scope = scope
	return "authorization-code", nil
}

func (c *cookieAuthClaudeClient) ExchangeCodeForToken(_ context.Context, _ string, _ string, _ string, _ string, isSetupToken bool) (*oauth.TokenResponse, error) {
	c.isSetupToken = isSetupToken
	return &oauth.TokenResponse{
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		TokenType:    "Bearer",
		ExpiresIn:    3600,
	}, nil
}

func (c *cookieAuthClaudeClient) RefreshToken(_ context.Context, _ string, _ string) (*oauth.TokenResponse, error) {
	panic("unexpected RefreshToken call")
}

func TestOAuthHandlerCookieAuthUsesFullScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := &cookieAuthClaudeClient{}
	oauthService := service.NewOAuthService(nil, client)
	defer oauthService.Stop()
	handler := NewOAuthHandler(oauthService)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/cookie-auth", strings.NewReader(`{"code":"sk-ant-sid01-redacted"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.CookieAuth(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "sk-ant-sid01-redacted", client.sessionKey)
	require.Equal(t, oauth.ScopeAPI, client.scope)
	require.False(t, client.isSetupToken)
	require.NotContains(t, recorder.Body.String(), "sk-ant-sid01-redacted")
}
