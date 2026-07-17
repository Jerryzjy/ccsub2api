package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	ClaudeWebBaseURL   = "https://claude.ai"
	ClaudeWebUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36"
	ClaudeWebSecCHUA   = `"Chromium";v="146", "Google Chrome";v="146", "Not_A Brand";v="99"`
)

var ErrClaudeWebProfileDecode = errors.New("decode Claude Web profile response")

type ClaudeWebTransport interface {
	Do(ctx context.Context, req *http.Request, proxyURL string, accountID int64) (*http.Response, error)
}

type ClaudeWebHTTPError struct {
	StatusCode int
	Header     http.Header
	Kind       ClaudeWebErrorKind
}

func (e *ClaudeWebHTTPError) Error() string {
	return fmt.Sprintf("claude web upstream returned status %d", e.StatusCode)
}

type ClaudeWebClient struct {
	transport ClaudeWebTransport
	baseURL   string
	now       func() time.Time
}

func NewClaudeWebClient(transport ClaudeWebTransport) *ClaudeWebClient {
	return &ClaudeWebClient{transport: transport, baseURL: ClaudeWebBaseURL, now: time.Now}
}

func (c *ClaudeWebClient) FetchProfile(ctx context.Context, account *Account, proxyURL string) (ClaudeWebProfile, error) {
	request, err := c.newRequest(ctx, account, http.MethodGet, "/api/account", nil, "/new")
	if err != nil {
		return ClaudeWebProfile{}, err
	}
	response, err := c.transport.Do(ctx, request, proxyURL, account.ID)
	if err != nil {
		return ClaudeWebProfile{}, fmt.Errorf("fetch Claude Web profile: %w", err)
	}
	defer response.Body.Close()
	if err := claudeWebRequireStatus(response, http.StatusOK); err != nil {
		return ClaudeWebProfile{}, err
	}
	var raw claudeWebAccountProfile
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&raw); err != nil {
		return ClaudeWebProfile{}, ErrClaudeWebProfileDecode
	}
	return normalizeClaudeWebProfile(raw), nil
}

func (c *ClaudeWebClient) ResolveOrganization(ctx context.Context, account *Account, proxyURL string) (string, error) {
	if account == nil || !account.IsClaudeWebSession() {
		return "", errors.New("invalid Claude Web account")
	}
	if cached := strings.TrimSpace(account.GetCredential(ClaudeWebCredentialOrganizationID)); cached != "" {
		return cached, nil
	}
	if rawCookie := strings.TrimSpace(account.GetCredential(ClaudeWebCredentialCookie)); rawCookie != "" {
		normalized, err := NormalizeClaudeWebCookie(rawCookie, c.now())
		if err != nil {
			return "", err
		}
		if normalized.OrganizationID != "" {
			return normalized.OrganizationID, nil
		}
	}
	request, err := c.newRequest(ctx, account, http.MethodGet, "/api/organizations", nil, "/new")
	if err != nil {
		return "", err
	}
	response, err := c.transport.Do(ctx, request, proxyURL, account.ID)
	if err != nil {
		return "", fmt.Errorf("resolve Claude Web organization: %w", err)
	}
	defer response.Body.Close()
	if err := claudeWebRequireStatus(response, http.StatusOK); err != nil {
		return "", err
	}
	var organizations []struct {
		UUID string `json:"uuid"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&organizations); err != nil {
		return "", errors.New("decode Claude Web organizations response")
	}
	if len(organizations) == 0 || strings.TrimSpace(organizations[0].UUID) == "" {
		return "", errors.New("no Claude Web organization found")
	}
	return organizations[0].UUID, nil
}

func (c *ClaudeWebClient) CreateConversation(ctx context.Context, account *Account, proxyURL, organizationID string) (string, error) {
	organizationID = strings.TrimSpace(organizationID)
	if organizationID == "" {
		return "", errors.New("organization id is required")
	}
	body := map[string]any{"name": "chat", "organization_uuid": organizationID}
	path := "/api/organizations/" + url.PathEscape(organizationID) + "/chat_conversations"
	request, err := c.newRequest(ctx, account, http.MethodPost, path, body, "/new")
	if err != nil {
		return "", err
	}
	response, err := c.transport.Do(ctx, request, proxyURL, account.ID)
	if err != nil {
		return "", fmt.Errorf("create Claude Web conversation: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusCreated {
		return "", claudeWebResponseError(response)
	}
	var conversation struct {
		UUID string `json:"uuid"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&conversation); err != nil {
		return "", errors.New("decode Claude Web conversation response")
	}
	if strings.TrimSpace(conversation.UUID) == "" {
		return "", errors.New("Claude Web conversation id is empty")
	}
	return conversation.UUID, nil
}

func (c *ClaudeWebClient) Complete(ctx context.Context, account *Account, proxyURL, organizationID, conversationID string, payload ClaudeWebCompletionRequest) (*http.Response, error) {
	path := "/api/organizations/" + url.PathEscape(organizationID) + "/chat_conversations/" + url.PathEscape(conversationID) + "/completion"
	request, err := c.newRequest(ctx, account, http.MethodPost, path, payload, "/chat/"+url.PathEscape(conversationID))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "text/event-stream")
	response, err := c.transport.Do(ctx, request, proxyURL, account.ID)
	if err != nil {
		return nil, fmt.Errorf("complete Claude Web conversation: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		err := claudeWebResponseError(response)
		response.Body.Close()
		return nil, err
	}
	return response, nil
}

func (c *ClaudeWebClient) DeleteConversation(ctx context.Context, account *Account, proxyURL, organizationID, conversationID string) error {
	path := "/api/organizations/" + url.PathEscape(organizationID) + "/chat_conversations/" + url.PathEscape(conversationID)
	request, err := c.newRequest(ctx, account, http.MethodDelete, path, nil, "/chat/"+url.PathEscape(conversationID))
	if err != nil {
		return err
	}
	response, err := c.transport.Do(ctx, request, proxyURL, account.ID)
	if err != nil {
		return fmt.Errorf("delete Claude Web conversation: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusNoContent {
		return claudeWebResponseError(response)
	}
	return nil
}

func (c *ClaudeWebClient) newRequest(ctx context.Context, account *Account, method, path string, body any, refererPath string) (*http.Request, error) {
	if c == nil || c.transport == nil {
		return nil, errors.New("Claude Web transport is not configured")
	}
	if account == nil || !account.IsClaudeWebSession() {
		return nil, errors.New("invalid Claude Web account")
	}
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, errors.New("encode Claude Web request")
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.baseURL, "/")+path, reader)
	if err != nil {
		return nil, errors.New("create Claude Web request")
	}
	cookieHeader, deviceID, err := c.accountCookie(account)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Cookie", cookieHeader)
	request.Header.Set("Origin", c.baseURL)
	request.Header.Set("Referer", strings.TrimRight(c.baseURL, "/")+refererPath)
	request.Header.Set("User-Agent", ClaudeWebUserAgent)
	request.Header.Set("sec-ch-ua", ClaudeWebSecCHUA)
	request.Header.Set("sec-ch-ua-mobile", "?0")
	request.Header.Set("sec-ch-ua-platform", `"Windows"`)
	request.Header.Set("sec-fetch-dest", "empty")
	request.Header.Set("sec-fetch-mode", "cors")
	request.Header.Set("sec-fetch-site", "same-origin")
	request.Header.Set("anthropic-client-platform", "web_claude_ai")
	request.Header.Set("anthropic-device-id", deviceID)
	return request, nil
}

func (c *ClaudeWebClient) accountCookie(account *Account) (header, deviceID string, err error) {
	raw := account.GetCredential(ClaudeWebCredentialCookie)
	sessionKey := account.GetCredential(ClaudeWebCredentialSessionKey)
	if strings.TrimSpace(raw) != "" {
		normalized, normalizeErr := NormalizeClaudeWebCookie(raw, c.now())
		if normalizeErr != nil {
			return "", "", normalizeErr
		}
		header = normalized.Header
		if normalized.SessionKey != "" {
			sessionKey = normalized.SessionKey
		}
	}
	if strings.TrimSpace(sessionKey) == "" {
		return "", "", errors.New("Claude Web sessionKey is missing")
	}
	deviceID = cookieValueFromHeader(header, "anthropic-device-id")
	if deviceID == "" {
		deviceID = deterministicClaudeWebDeviceID(sessionKey)
		header = appendCookieHeader(header, "anthropic-device-id", deviceID)
	}
	if cookieValueFromHeader(header, "sessionKey") == "" {
		header = appendCookieHeader(header, "sessionKey", sessionKey)
	}
	if cookieValueFromHeader(header, "sessionKeyLC") == "" {
		header = appendCookieHeader(header, "sessionKeyLC", fmt.Sprintf("%d", c.now().UnixMilli()))
	}
	normalized, normalizeErr := NormalizeClaudeWebCookie(header, c.now())
	if normalizeErr != nil {
		return "", "", normalizeErr
	}
	return normalized.Header, deviceID, nil
}

func deterministicClaudeWebDeviceID(sessionKey string) string {
	digest := sha256.Sum256([]byte(sessionKey))
	hexValue := hex.EncodeToString(digest[:16])
	return hexValue[0:8] + "-" + hexValue[8:12] + "-" + hexValue[12:16] + "-" + hexValue[16:20] + "-" + hexValue[20:32]
}

func cookieValueFromHeader(header, name string) string {
	for _, part := range strings.Split(header, ";") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok && key == name {
			return value
		}
	}
	return ""
}

func appendCookieHeader(header, name, value string) string {
	if strings.TrimSpace(header) == "" {
		return name + "=" + value
	}
	return header + "; " + name + "=" + value
}

func claudeWebRequireStatus(response *http.Response, allowed ...int) error {
	for _, status := range allowed {
		if response.StatusCode == status {
			return nil
		}
	}
	return claudeWebResponseError(response)
}

func claudeWebResponseError(response *http.Response) error {
	if response == nil {
		return errors.New("Claude Web upstream returned no response")
	}
	body, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	return &ClaudeWebHTTPError{
		StatusCode: response.StatusCode,
		Header:     response.Header.Clone(),
		Kind:       ClassifyClaudeWebResponse(response.StatusCode, response.Header, body),
	}
}
