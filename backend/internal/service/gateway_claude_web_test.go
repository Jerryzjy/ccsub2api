package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type claudeWebConversationCacheStub struct {
	GatewayCache
	mu       sync.Mutex
	values   map[string][]byte
	getCalls int
	getHits  int
	setCalls int
}

func (s *claudeWebConversationCacheStub) GetClaudeWebConversation(_ context.Context, key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.getCalls++
	value := s.values[key]
	if len(value) > 0 {
		s.getHits++
	}
	return append([]byte(nil), value...), nil
}

func (s *claudeWebConversationCacheStub) SetClaudeWebConversation(_ context.Context, key string, value []byte, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.values == nil {
		s.values = make(map[string][]byte)
	}
	s.setCalls++
	s.values[key] = append([]byte(nil), value...)
	return nil
}

func (s *claudeWebConversationCacheStub) DeleteClaudeWebConversation(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.values, key)
	return nil
}

func (s *claudeWebConversationCacheStub) TryLockClaudeWebConversation(_ context.Context, _, _ string, _ time.Duration) (bool, error) {
	return true, nil
}

func (s *claudeWebConversationCacheStub) UnlockClaudeWebConversation(_ context.Context, _, _ string) error {
	return nil
}

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
	require.Equal(t, "Claude Web is unavailable from the current outbound region; bind or replace the account proxy", claudeWebPublicErrorMessage(ClaudeWebErrorRegionBlocked))
	require.Equal(t, ClaudeWebErrorUpstream, ClassifyClaudeWebResponse(http.StatusBadGateway, nil, nil))
}

func TestClaudeWebPublicErrorFromBody(t *testing.T) {
	body := []byte(`{"type":"error","error":{"type":"web_session_region_blocked","message":"Claude Web is unavailable from the current outbound region; bind or replace the account proxy"}}`)

	kind, message, ok := ClaudeWebPublicErrorFromBody(body)

	require.True(t, ok)
	require.Equal(t, ClaudeWebErrorRegionBlocked, kind)
	require.Equal(t, claudeWebPublicErrorMessage(ClaudeWebErrorRegionBlocked), message)

	_, _, ok = ClaudeWebPublicErrorFromBody([]byte(`{"error":{"type":"other","message":"do not expose me"}}`))
	require.False(t, ok)
}

func TestTranslateClaudeWebSSEDoesNotCommitBeforeInitialUpstreamError(t *testing.T) {
	input := "event: error\n" +
		`data: {"type":"error","error":{"type":"rate_limit_error","message":"rate limit reached"}}` + "\n\n"
	var output strings.Builder

	_, err := TranslateClaudeWebSSE(strings.NewReader(input), &output, ClaudeWebStreamMeta{
		Model: "claude-haiku-4-5", MessageID: "msg-test", InputTokens: 1,
	})

	var streamErr *ClaudeWebStreamError
	require.ErrorAs(t, err, &streamErr)
	require.Equal(t, ClaudeWebErrorRateLimited, streamErr.Kind)
	require.Empty(t, output.String())
}

func TestClaudeWebForwardErrorConvertsStreamError(t *testing.T) {
	err := claudeWebForwardError(&ClaudeWebStreamError{Kind: ClaudeWebErrorRateLimited})

	var failover *UpstreamFailoverError
	require.ErrorAs(t, err, &failover)
	require.Equal(t, http.StatusTooManyRequests, failover.StatusCode)
	kind, message, ok := ClaudeWebPublicErrorFromBody(failover.ResponseBody)
	require.True(t, ok)
	require.Equal(t, ClaudeWebErrorRateLimited, kind)
	require.Equal(t, claudeWebPublicErrorMessage(kind), message)
}

func TestClaudeWebForwardErrorConvertsPlainUpstreamFailure(t *testing.T) {
	err := claudeWebForwardError(errors.New("dial failed: sessionKey=secret-value"))

	var failover *UpstreamFailoverError
	require.ErrorAs(t, err, &failover)
	require.Equal(t, http.StatusBadGateway, failover.StatusCode)
	kind, message, ok := ClaudeWebPublicErrorFromBody(failover.ResponseBody)
	require.True(t, ok)
	require.Equal(t, ClaudeWebErrorUpstream, kind)
	require.Equal(t, claudeWebPublicErrorMessage(kind), message)
	require.NotContains(t, string(failover.ResponseBody), "secret-value")
}

func TestClaudeWebForwardErrorPreservesContextCancellation(t *testing.T) {
	require.ErrorIs(t, claudeWebForwardError(context.Canceled), context.Canceled)
	require.ErrorIs(t, claudeWebForwardError(context.DeadlineExceeded), context.DeadlineExceeded)
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

func TestGatewayForwardClaudeWebSessionRecordsConversationCacheCreationAndRead(t *testing.T) {
	gin.SetMode(gin.TestMode)
	firstSSE := "event: content_block_delta\n" +
		"data: {\"delta\":{\"type\":\"text_delta\",\"text\":\"first answer\"}}\n\n" +
		"event: message_stop\ndata: {}\n\n"
	secondSSE := "event: content_block_delta\n" +
		"data: {\"delta\":{\"type\":\"text_delta\",\"text\":\"second answer\"}}\n\n" +
		"event: message_stop\ndata: {}\n\n"
	transport := &claudeWebTransportStub{responses: []*http.Response{
		claudeWebTestResponse(http.StatusCreated, `{"uuid":"conv-cache"}`),
		claudeWebTestResponse(http.StatusOK, firstSSE),
		claudeWebTestResponse(http.StatusOK, secondSSE),
	}}
	cache := &claudeWebConversationCacheStub{values: make(map[string][]byte)}
	service := &GatewayService{
		cfg:             &config.Config{},
		cache:           cache,
		claudeWebClient: NewClaudeWebClient(transport),
	}
	account := &Account{
		ID:       26,
		Platform: PlatformAnthropic,
		Type:     AccountTypeWebSession,
		Credentials: map[string]any{
			ClaudeWebCredentialCookie:         "sessionKey=test",
			ClaudeWebCredentialOrganizationID: "org-1",
		},
	}

	first := mustParseClaudeWebConversationRequest(t, `{
		"model":"claude-sonnet-5","stream":false,
		"system":[{"type":"text","text":"be concise","cache_control":{"type":"ephemeral"}}],
		"messages":[{"role":"user","content":"first question"}]
	}`)
	first.SessionContext = &SessionContext{APIKeyID: 9, ClientIP: "127.0.0.1", UserAgent: "test"}
	firstRecorder := httptest.NewRecorder()
	firstCtx, _ := gin.CreateTestContext(firstRecorder)
	firstCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	firstResult, err := service.Forward(context.Background(), firstCtx, account, first)

	require.NoError(t, err)
	require.Zero(t, firstResult.Usage.InputTokens)
	require.Greater(t, firstResult.Usage.CacheCreationInputTokens, 0)
	require.Equal(t, firstResult.Usage.CacheCreationInputTokens, firstResult.Usage.CacheCreation5mTokens)
	require.Zero(t, firstResult.Usage.CacheReadInputTokens)
	require.Contains(t, firstRecorder.Body.String(), `"cache_creation_input_tokens":`)

	second := mustParseClaudeWebConversationRequest(t, `{
		"model":"claude-sonnet-5","stream":false,
		"system":[{"type":"text","text":"be concise","cache_control":{"type":"ephemeral"}}],
		"messages":[
			{"role":"user","content":"first question"},
			{"role":"assistant","content":[{"type":"text","text":"first answer"}]},
			{"role":"user","content":"second question"}
		]
	}`)
	second.SessionContext = &SessionContext{APIKeyID: 9, ClientIP: "127.0.0.1", UserAgent: "test"}
	secondRecorder := httptest.NewRecorder()
	secondCtx, _ := gin.CreateTestContext(secondRecorder)
	secondCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	secondResult, err := service.Forward(context.Background(), secondCtx, account, second)

	require.NoError(t, err)
	require.Zero(t, secondResult.Usage.InputTokens)
	require.Greater(t, secondResult.Usage.CacheCreationInputTokens, 0)
	require.Greater(t, secondResult.Usage.CacheReadInputTokens, 0)
	require.Contains(t, secondRecorder.Body.String(), `"cache_read_input_tokens":`)
	require.Equal(t, 2, cache.getCalls)
	require.Equal(t, 1, cache.getHits)
	require.Equal(t, 2, cache.setCalls)
	require.Len(t, transport.requests, 3)
	require.Equal(t, "/api/organizations/org-1/chat_conversations/conv-cache/completion", transport.requests[2].URL.Path)
	var reusedPayload ClaudeWebCompletionRequest
	require.NoError(t, json.NewDecoder(transport.requests[2].Body).Decode(&reusedPayload))
	require.Equal(t, "second question", reusedPayload.Prompt)
	require.NotEmpty(t, reusedPayload.ParentMessageUUID)
}

func TestGatewayForwardClaudeWebSessionCredentialRotationDoesNotReuseOldConversation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	firstSSE := "event: content_block_delta\n" +
		"data: {\"delta\":{\"type\":\"text_delta\",\"text\":\"first answer\"}}\n\n" +
		"event: message_stop\ndata: {}\n\n"
	secondSSE := "event: content_block_delta\n" +
		"data: {\"delta\":{\"type\":\"text_delta\",\"text\":\"second answer\"}}\n\n" +
		"event: message_stop\ndata: {}\n\n"
	transport := &claudeWebTransportStub{responses: []*http.Response{
		claudeWebTestResponse(http.StatusCreated, `{"uuid":"conv-old"}`),
		claudeWebTestResponse(http.StatusOK, firstSSE),
		claudeWebTestResponse(http.StatusCreated, `{"uuid":"conv-new"}`),
		claudeWebTestResponse(http.StatusOK, secondSSE),
	}}
	cache := &claudeWebConversationCacheStub{values: make(map[string][]byte)}
	service := &GatewayService{
		cfg:             &config.Config{},
		cache:           cache,
		claudeWebClient: NewClaudeWebClient(transport),
	}
	account := &Account{
		ID:       27,
		Platform: PlatformAnthropic,
		Type:     AccountTypeWebSession,
		Credentials: map[string]any{
			ClaudeWebCredentialSessionKey:     "old-session-key",
			ClaudeWebCredentialOrganizationID: "org-1",
		},
	}

	first := mustParseClaudeWebConversationRequest(t, `{
		"model":"claude-sonnet-5","stream":false,
		"messages":[{"role":"user","content":"first question"}]
	}`)
	first.SessionContext = &SessionContext{APIKeyID: 9, ClientIP: "127.0.0.1", UserAgent: "test"}
	firstRecorder := httptest.NewRecorder()
	firstCtx, _ := gin.CreateTestContext(firstRecorder)
	firstCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	_, err := service.Forward(context.Background(), firstCtx, account, first)
	require.NoError(t, err)

	account.Credentials[ClaudeWebCredentialSessionKey] = "new-session-key"
	second := mustParseClaudeWebConversationRequest(t, `{
		"model":"claude-sonnet-5","stream":false,
		"messages":[
			{"role":"user","content":"first question"},
			{"role":"assistant","content":"first answer"},
			{"role":"user","content":"second question"}
		]
	}`)
	second.SessionContext = &SessionContext{APIKeyID: 9, ClientIP: "127.0.0.1", UserAgent: "test"}
	secondRecorder := httptest.NewRecorder()
	secondCtx, _ := gin.CreateTestContext(secondRecorder)
	secondCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	_, err = service.Forward(context.Background(), secondCtx, account, second)

	require.NoError(t, err)
	require.Len(t, transport.requests, 4)
	require.Equal(t, "/api/organizations/org-1/chat_conversations", transport.requests[2].URL.Path)
	require.Contains(t, transport.requests[2].Header.Get("Cookie"), "sessionKey=new-session-key")
	require.Contains(t, secondRecorder.Body.String(), "second answer")
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
		claudeWebTestResponse(http.StatusOK, `{"email_address":"web@example.com","memberships":[{"seat_tier":"pro"}]}`),
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
	require.Len(t, transport.requests, 4)
	require.Equal(t, "/api/account", transport.requests[0].URL.Path)
}

func TestProbeClaudeWebSessionAccount(t *testing.T) {
	transport := &claudeWebTransportStub{responses: []*http.Response{
		claudeWebTestResponse(http.StatusOK, `{"email_address":"probe@example.com","memberships":[{"seat_tier":"pro"}]}`),
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
	require.Len(t, transport.requests, 3)
	require.Equal(t, http.MethodGet, transport.requests[0].Method)
	require.Equal(t, "/api/account", transport.requests[0].URL.Path)
	require.Equal(t, http.MethodPost, transport.requests[1].Method)
	require.Equal(t, http.MethodDelete, transport.requests[2].Method)
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
