package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestClassifyClaudeWebResponse(t *testing.T) {
	require.Equal(t, ClaudeWebErrorExpired, ClassifyClaudeWebResponse(http.StatusUnauthorized, nil, nil))
	require.Equal(t, ClaudeWebErrorCloudflare, ClassifyClaudeWebResponse(
		http.StatusForbidden,
		http.Header{"Cf-Mitigated": []string{"challenge"}},
		[]byte("<html>challenge</html>"),
	))
	require.Equal(t, ClaudeWebErrorRateLimited, ClassifyClaudeWebResponse(http.StatusTooManyRequests, nil, nil))
	require.Equal(t, ClaudeWebErrorRegionBlocked, ClassifyClaudeWebResponse(
		http.StatusFound,
		http.Header{"Location": []string{"https://www.anthropic.com/app-unavailable-in-region"}},
		nil,
	))
	require.Equal(t, "Claude Web is unavailable from the account proxy region", claudeWebPublicErrorMessage(ClaudeWebErrorRegionBlocked))
	require.Equal(t, ClaudeWebErrorUpstream, ClassifyClaudeWebResponse(http.StatusBadGateway, nil, nil))
}

func TestGatewayForwardClaudeWebSessionUsesWebTransport(t *testing.T) {
	gin.SetMode(gin.TestMode)
	completionSSE := "event: content_block_delta\n" +
		"data: {\"delta\":{\"type\":\"text_delta\",\"text\":\"pong\"}}\n\n" +
		"event: message_stop\ndata: {}\n\n"
	transport := &claudeWebTransportStub{responses: []*http.Response{
		claudeWebTestResponse(http.StatusCreated, `{"uuid":"conv-1"}`),
		claudeWebTestResponse(http.StatusOK, completionSSE),
		claudeWebTestResponse(http.StatusNoContent, ""),
	}}
	service := &GatewayService{
		cfg:             &config.Config{},
		claudeWebClient: NewClaudeWebClient(transport),
	}
	account := &Account{
		ID:          21,
		Name:        "web",
		Platform:    PlatformAnthropic,
		Type:        AccountTypeWebSession,
		Concurrency: 2,
		Credentials: map[string]any{
			ClaudeWebCredentialCookie:         "sessionKey=test",
			ClaudeWebCredentialOrganizationID: "org-1",
		},
	}
	body := []byte(`{"model":"claude-sonnet-4-5","stream":true,"max_tokens":128,"messages":[{"role":"user","content":"ping"}]}`)
	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	result, err := service.Forward(context.Background(), ctx, account, parsed)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Stream)
	require.Equal(t, "claude-sonnet-4-5", result.Model)
	require.Greater(t, result.Usage.OutputTokens, 0)
	require.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
	require.Contains(t, recorder.Body.String(), "event: message_start")
	require.Contains(t, recorder.Body.String(), `"text":"pong"`)
	require.Contains(t, recorder.Body.String(), "event: message_stop")
	require.Len(t, transport.requests, 3)
	require.Equal(t, "/api/organizations/org-1/chat_conversations", transport.requests[0].URL.Path)
	require.Equal(t, "/api/organizations/org-1/chat_conversations/conv-1/completion", transport.requests[1].URL.Path)
	require.Equal(t, http.MethodDelete, transport.requests[2].Method)
}

func TestGatewayForwardClaudeWebSessionReturnsFailoverError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	transport := &claudeWebTransportStub{responses: []*http.Response{
		claudeWebTestResponse(http.StatusCreated, `{"uuid":"conv-1"}`),
		claudeWebTestResponse(http.StatusUnauthorized, "sessionKey=secret-upstream-value"),
	}}
	service := &GatewayService{cfg: &config.Config{}, claudeWebClient: NewClaudeWebClient(transport)}
	account := &Account{ID: 22, Platform: PlatformAnthropic, Type: AccountTypeWebSession, Credentials: map[string]any{
		ClaudeWebCredentialCookie:         "sessionKey=test",
		ClaudeWebCredentialOrganizationID: "org-1",
	}}
	body := []byte(`{"model":"claude-sonnet-4-5","stream":false,"messages":[{"role":"user","content":"ping"}]}`)
	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	_, err = service.Forward(context.Background(), ctx, account, parsed)

	var failover *UpstreamFailoverError
	require.ErrorAs(t, err, &failover)
	require.Equal(t, http.StatusUnauthorized, failover.StatusCode)
	require.NotContains(t, string(failover.ResponseBody), "secret-upstream-value")
}

func TestClaudeWebAccountConnection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	completionSSE := "event: content_block_delta\n" +
		"data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"ok\"}}\n\n" +
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
	transport := &claudeWebTransportStub{responses: []*http.Response{
		claudeWebTestResponse(http.StatusCreated, `{"uuid":"conv-test"}`),
		claudeWebTestResponse(http.StatusOK, completionSSE),
		claudeWebTestResponse(http.StatusNoContent, ""),
	}}
	testService := &AccountTestService{claudeWebClient: NewClaudeWebClient(transport)}
	account := &Account{ID: 23, Platform: PlatformAnthropic, Type: AccountTypeWebSession, Credentials: map[string]any{
		ClaudeWebCredentialCookie:         "sessionKey=test",
		ClaudeWebCredentialOrganizationID: "org-1",
	}}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/admin/accounts/23/test", nil)

	err := testService.testClaudeAccountConnection(ctx, account, "claude-sonnet-4-5")

	require.NoError(t, err)
	require.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
	require.Contains(t, recorder.Body.String(), `"type":"test_start"`)
	require.Contains(t, recorder.Body.String(), `"type":"content","text":"ok"`)
	require.Contains(t, recorder.Body.String(), `"type":"test_complete"`)
	require.Len(t, transport.requests, 3)
}

func TestProbeClaudeWebSessionAccount(t *testing.T) {
	transport := &claudeWebTransportStub{responses: []*http.Response{
		claudeWebTestResponse(http.StatusCreated, `{"uuid":"conv-probe"}`),
		claudeWebTestResponse(http.StatusNoContent, ""),
	}}
	testService := &AccountTestService{claudeWebClient: NewClaudeWebClient(transport)}
	account := &Account{ID: 24, Platform: PlatformAnthropic, Type: AccountTypeWebSession, Credentials: map[string]any{
		ClaudeWebCredentialCookie:         "sessionKey=test",
		ClaudeWebCredentialOrganizationID: "org-probe",
	}}

	organizationID, err := testService.probeClaudeWebSessionAccount(context.Background(), account)

	require.NoError(t, err)
	require.Equal(t, "org-probe", organizationID)
	require.Len(t, transport.requests, 2)
	require.Equal(t, http.MethodPost, transport.requests[0].Method)
	require.Equal(t, http.MethodDelete, transport.requests[1].Method)
}

func TestForwardCountTokensClaudeWebSessionUsesLocalEstimate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &GatewayService{cfg: &config.Config{}}
	account := &Account{ID: 25, Platform: PlatformAnthropic, Type: AccountTypeWebSession, Credentials: map[string]any{
		ClaudeWebCredentialCookie: "sessionKey=test",
	}}
	body := []byte(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"count this text"}]}`)
	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", nil)

	err = service.ForwardCountTokens(context.Background(), ctx, account, parsed)

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		InputTokens int `json:"input_tokens"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Greater(t, response.InputTokens, 0)
}
