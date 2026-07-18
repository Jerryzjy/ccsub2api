package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/oauth"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type claudeCookieOAuthClientStub struct {
	scope            string
	sessionKey       string
	cookieHeader     string
	authorizationErr error
}

func (s *claudeCookieOAuthClientStub) GetOrganizationUUID(_ context.Context, sessionKey, cookieHeader, _ string) (string, error) {
	s.sessionKey = sessionKey
	s.cookieHeader = cookieHeader
	return "org-1", nil
}

func (s *claudeCookieOAuthClientStub) GetAuthorizationCode(_ context.Context, _, _, _ string, scope string, _, _, _ string) (string, error) {
	s.scope = scope
	if s.authorizationErr != nil {
		return "", s.authorizationErr
	}
	return "auth-code", nil
}

func TestAccountHandlerCreateClaudeCookieOAuthReportsAuthorizationStageWithoutSecrets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adminSvc := newStubAdminService()
	oauthClient := &claudeCookieOAuthClientStub{authorizationErr: errors.New("upstream rejected secret-cookie-value")}
	oauthSvc := service.NewOAuthService(nil, oauthClient)
	defer oauthSvc.Stop()
	handler := NewAccountHandler(adminSvc, oauthSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.POST("/api/v1/admin/accounts/claude-cookie-oauth", handler.CreateClaudeCookieOAuth)

	body, err := json.Marshal(map[string]any{"name": "Cookie Account", "session_key": "secret-session-key"})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/claude-cookie-oauth", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), `"reason":"claude_cookie_oauth_failed"`)
	require.Contains(t, recorder.Body.String(), `"stage":"authorization"`)
	require.NotContains(t, recorder.Body.String(), "secret-session-key")
	require.NotContains(t, recorder.Body.String(), "secret-cookie-value")
	require.Empty(t, adminSvc.createdAccounts)
}

func (s *claudeCookieOAuthClientStub) ExchangeCodeForToken(context.Context, string, string, string, string, bool) (*oauth.TokenResponse, error) {
	return &oauth.TokenResponse{
		AccessToken:  "access-secret",
		RefreshToken: "refresh-secret",
		TokenType:    "Bearer",
		ExpiresIn:    28800,
		Scope:        oauth.ScopeClaudeAI,
	}, nil
}

func (s *claudeCookieOAuthClientStub) RefreshToken(context.Context, string, string) (*oauth.TokenResponse, error) {
	panic("not used")
}

func TestAccountHandlerCreateClaudeCookieOAuthFromSessionKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adminSvc := newStubAdminService()
	oauthClient := &claudeCookieOAuthClientStub{}
	oauthSvc := service.NewOAuthService(nil, oauthClient)
	defer oauthSvc.Stop()
	handler := NewAccountHandler(adminSvc, oauthSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.POST("/api/v1/admin/accounts/claude-cookie-oauth", handler.CreateClaudeCookieOAuth)

	body, err := json.Marshal(map[string]any{
		"name":        "Claude OAuth",
		"session_key": "session-secret",
		"concurrency": 2,
		"priority":    10,
	})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/claude-cookie-oauth", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Equal(t, oauth.ScopeClaudeAI, oauthClient.scope)
	require.Equal(t, "session-secret", oauthClient.sessionKey)
	require.Len(t, adminSvc.createdAccounts, 1)
	created := adminSvc.createdAccounts[0]
	require.Equal(t, service.PlatformAnthropic, created.Platform)
	require.Equal(t, service.AccountTypeOAuth, created.Type)
	require.Equal(t, "access-secret", created.Credentials["access_token"])
	require.Equal(t, "refresh-secret", created.Credentials["refresh_token"])
	require.NotContains(t, created.Credentials, "session_key")
	require.NotContains(t, recorder.Body.String(), "session-secret")
	require.NotContains(t, recorder.Body.String(), "access-secret")
	require.NotContains(t, recorder.Body.String(), "refresh-secret")
}

func TestAccountHandlerCreateClaudeCookieOAuthFromNetscapeCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adminSvc := newStubAdminService()
	oauthClient := &claudeCookieOAuthClientStub{}
	oauthSvc := service.NewOAuthService(nil, oauthClient)
	defer oauthSvc.Stop()
	handler := NewAccountHandler(adminSvc, oauthSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.POST("/api/v1/admin/accounts/claude-cookie-oauth", handler.CreateClaudeCookieOAuth)

	cookie := ".claude.ai\tTRUE\t/\tTRUE\t4102444800\t__ssid\tsecurity-cookie\n" +
		".claude.ai\tTRUE\t/\tTRUE\t4102444800\tsessionKey\tcookie-session-secret"
	body, err := json.Marshal(map[string]any{"name": "Cookie Account", "cookie": cookie})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/claude-cookie-oauth", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Equal(t, "cookie-session-secret", oauthClient.sessionKey)
	require.Contains(t, oauthClient.cookieHeader, "__ssid=security-cookie")
	require.Contains(t, oauthClient.cookieHeader, "sessionKey=cookie-session-secret")
	require.Len(t, adminSvc.createdAccounts, 1)
	require.NotContains(t, recorder.Body.String(), "cookie-session-secret")
}
