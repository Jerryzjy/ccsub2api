package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type claudeWebTransportStub struct {
	requests   []*http.Request
	proxyURLs  []string
	accountIDs []int64
	responses  []*http.Response
	err        error
}

func (s *claudeWebTransportStub) Do(_ context.Context, req *http.Request, proxyURL string, accountID int64) (*http.Response, error) {
	s.requests = append(s.requests, req)
	s.proxyURLs = append(s.proxyURLs, proxyURL)
	s.accountIDs = append(s.accountIDs, accountID)
	if s.err != nil {
		return nil, s.err
	}
	if len(s.responses) == 0 {
		return claudeWebTestResponse(http.StatusOK, `{}`), nil
	}
	resp := s.responses[0]
	s.responses = s.responses[1:]
	return resp, nil
}

func claudeWebTestResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestClaudeWebClientResolveOrganization(t *testing.T) {
	transport := &claudeWebTransportStub{responses: []*http.Response{
		claudeWebTestResponse(http.StatusOK, `[{"uuid":"org-1"}]`),
	}}
	client := NewClaudeWebClient(transport)
	account := &Account{
		ID:          7,
		Platform:    PlatformAnthropic,
		Type:        AccountTypeWebSession,
		Concurrency: 3,
		Credentials: map[string]any{
			ClaudeWebCredentialCookie: "sessionKey=test; foo=bar",
		},
	}

	got, err := client.ResolveOrganization(context.Background(), account, "socks5://proxy:1080")

	require.NoError(t, err)
	require.Equal(t, "org-1", got)
	require.Len(t, transport.requests, 1)
	require.Equal(t, "https://claude.ai/api/organizations", transport.requests[0].URL.String())
	cookieHeader := transport.requests[0].Header.Get("Cookie")
	require.Contains(t, cookieHeader, "foo=bar")
	require.Contains(t, cookieHeader, "sessionKey=test")
	require.Contains(t, cookieHeader, "sessionKeyLC=")
	require.Contains(t, cookieHeader, "anthropic-device-id=")
	require.Equal(t, ClaudeWebUserAgent, transport.requests[0].Header.Get("User-Agent"))
	require.Equal(t, ClaudeWebSecCHUA, transport.requests[0].Header.Get("sec-ch-ua"))
	require.Equal(t, "socks5://proxy:1080", transport.proxyURLs[0])
	require.Equal(t, int64(7), transport.accountIDs[0])
}

func TestClaudeWebClientResolveOrganizationUsesCachedCredential(t *testing.T) {
	transport := &claudeWebTransportStub{}
	client := NewClaudeWebClient(transport)
	account := &Account{
		ID:       8,
		Platform: PlatformAnthropic,
		Type:     AccountTypeWebSession,
		Credentials: map[string]any{
			ClaudeWebCredentialCookie:         "sessionKey=test",
			ClaudeWebCredentialOrganizationID: "org-cached",
		},
	}

	got, err := client.ResolveOrganization(context.Background(), account, "")

	require.NoError(t, err)
	require.Equal(t, "org-cached", got)
	require.Empty(t, transport.requests)
}

func TestClaudeWebClientCreateConversation(t *testing.T) {
	transport := &claudeWebTransportStub{responses: []*http.Response{
		claudeWebTestResponse(http.StatusCreated, `{"uuid":"conv-1"}`),
	}}
	client := NewClaudeWebClient(transport)
	account := &Account{
		ID:          9,
		Platform:    PlatformAnthropic,
		Type:        AccountTypeWebSession,
		Credentials: map[string]any{ClaudeWebCredentialSessionKey: "sk-test"},
	}

	got, err := client.CreateConversation(context.Background(), account, "", "org-1")

	require.NoError(t, err)
	require.Equal(t, "conv-1", got)
	require.Len(t, transport.requests, 1)
	require.Equal(t, http.MethodPost, transport.requests[0].Method)
	require.Equal(t, "https://claude.ai/api/organizations/org-1/chat_conversations", transport.requests[0].URL.String())
	require.Contains(t, transport.requests[0].Header.Get("Cookie"), "sessionKey=sk-test")
	var body map[string]any
	require.NoError(t, json.NewDecoder(transport.requests[0].Body).Decode(&body))
	require.Equal(t, "org-1", body["organization_uuid"])
}

func TestClaudeWebClientCompleteAndDeleteConversation(t *testing.T) {
	completion := claudeWebTestResponse(http.StatusOK, "event: message_stop\ndata: {}\n\n")
	transport := &claudeWebTransportStub{responses: []*http.Response{
		completion,
		claudeWebTestResponse(http.StatusNoContent, ""),
	}}
	client := NewClaudeWebClient(transport)
	account := &Account{
		ID:          10,
		Platform:    PlatformAnthropic,
		Type:        AccountTypeWebSession,
		Credentials: map[string]any{ClaudeWebCredentialCookie: "sessionKey=test"},
	}
	payload := ClaudeWebCompletionRequest{Prompt: "hello", Model: "claude-sonnet-4-5"}

	resp, err := client.Complete(context.Background(), account, "http://proxy:8080", "org-1", "conv-1", payload)
	require.NoError(t, err)
	require.Same(t, completion, resp)
	require.Equal(t, "https://claude.ai/api/organizations/org-1/chat_conversations/conv-1/completion", transport.requests[0].URL.String())
	require.Equal(t, "https://claude.ai/chat/conv-1", transport.requests[0].Header.Get("Referer"))
	require.Equal(t, "http://proxy:8080", transport.proxyURLs[0])

	require.NoError(t, client.DeleteConversation(context.Background(), account, "http://proxy:8080", "org-1", "conv-1"))
	require.Equal(t, http.MethodDelete, transport.requests[1].Method)
	require.Equal(t, "https://claude.ai/api/organizations/org-1/chat_conversations/conv-1", transport.requests[1].URL.String())
}

func TestClaudeWebClientRejectsUnexpectedStatusWithoutLeakingBody(t *testing.T) {
	transport := &claudeWebTransportStub{responses: []*http.Response{
		claudeWebTestResponse(http.StatusUnauthorized, `secret-cookie-value`),
	}}
	client := NewClaudeWebClient(transport)
	account := &Account{ID: 11, Platform: PlatformAnthropic, Type: AccountTypeWebSession, Credentials: map[string]any{
		ClaudeWebCredentialCookie: "sessionKey=secret-cookie-value",
	}}

	_, err := client.ResolveOrganization(context.Background(), account, "")

	require.Error(t, err)
	require.NotContains(t, err.Error(), "secret-cookie-value")
	require.Contains(t, err.Error(), "status 401")
}

func TestClaudeWebClientClassifiesCloudflareHTMLWithoutRetainingBody(t *testing.T) {
	transport := &claudeWebTransportStub{responses: []*http.Response{
		claudeWebTestResponse(http.StatusForbidden, `<html><title>Just a moment...</title><div>Cloudflare challenge</div></html>`),
	}}
	client := NewClaudeWebClient(transport)
	account := &Account{ID: 12, Platform: PlatformAnthropic, Type: AccountTypeWebSession, Credentials: map[string]any{
		ClaudeWebCredentialCookie: "sessionKey=test",
	}}

	_, err := client.ResolveOrganization(context.Background(), account, "")

	var upstreamErr *ClaudeWebHTTPError
	require.ErrorAs(t, err, &upstreamErr)
	require.Equal(t, ClaudeWebErrorCloudflare, upstreamErr.Kind)
	require.NotContains(t, err.Error(), "Cloudflare challenge")
}
