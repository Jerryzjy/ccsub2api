package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type ClaudeWebErrorKind string

const (
	ClaudeWebErrorExpired     ClaudeWebErrorKind = "web_session_expired"
	ClaudeWebErrorCloudflare  ClaudeWebErrorKind = "web_session_cloudflare"
	ClaudeWebErrorRateLimited ClaudeWebErrorKind = "web_session_rate_limited"
	ClaudeWebErrorUpstream    ClaudeWebErrorKind = "web_session_upstream"
)

func ClassifyClaudeWebResponse(status int, header http.Header, body []byte) ClaudeWebErrorKind {
	switch status {
	case http.StatusUnauthorized:
		return ClaudeWebErrorExpired
	case http.StatusForbidden:
		if strings.EqualFold(strings.TrimSpace(header.Get("cf-mitigated")), "challenge") {
			return ClaudeWebErrorCloudflare
		}
		lowerBody := strings.ToLower(string(body))
		if strings.Contains(lowerBody, "cf-chl") || strings.Contains(lowerBody, "cloudflare") || strings.Contains(lowerBody, "challenge") {
			return ClaudeWebErrorCloudflare
		}
		return ClaudeWebErrorExpired
	case http.StatusTooManyRequests:
		return ClaudeWebErrorRateLimited
	default:
		return ClaudeWebErrorUpstream
	}
}

func (s *GatewayService) forwardClaudeWebSession(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	parsed *ParsedRequest,
	startTime time.Time,
) (*ForwardResult, error) {
	if s.claudeWebClient == nil {
		return nil, errors.New("Claude Web client is not configured")
	}
	upstreamModel := ResolveClaudeWebModel(account.GetMappedModel(parsed.Model))
	payload, err := BuildClaudeWebCompletion(parsed.Body.Bytes(), upstreamModel)
	if err != nil {
		return nil, err
	}
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	if err := s.enforceProxyRequirement(account, proxyURL); err != nil {
		return nil, err
	}

	organizationID, err := s.claudeWebClient.ResolveOrganization(ctx, account, proxyURL)
	if err != nil {
		return nil, claudeWebForwardError(err)
	}
	conversationID, err := s.claudeWebClient.CreateConversation(ctx, account, proxyURL, organizationID)
	if err != nil {
		return nil, claudeWebForwardError(err)
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		_ = s.claudeWebClient.DeleteConversation(cleanupCtx, account, proxyURL, organizationID, conversationID)
	}()

	response, err := s.claudeWebClient.Complete(ctx, account, proxyURL, organizationID, conversationID, payload)
	if err != nil {
		return nil, claudeWebForwardError(err)
	}
	defer response.Body.Close()
	if parsed.OnUpstreamAccepted != nil {
		parsed.OnUpstreamAccepted()
	}

	messageID := "msg_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	inputTokens := estimateClaudeWebTokens(utf8.RuneCountInString(payload.Prompt))
	var usage ClaudeUsage
	firstTokenValue := int(time.Since(startTime).Milliseconds())
	firstTokenMs := &firstTokenValue
	clientDisconnect := false

	if parsed.Stream {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")
		usage, err = TranslateClaudeWebSSE(response.Body, claudeWebFlushWriter{writer: c.Writer}, ClaudeWebStreamMeta{
			Model: parsed.Model, MessageID: messageID, InputTokens: inputTokens,
		})
		if err != nil {
			if ctx.Err() != nil {
				clientDisconnect = true
			}
			return nil, err
		}
	} else {
		var body []byte
		body, usage, err = AggregateClaudeWebSSE(response.Body, parsed.Model, messageID, inputTokens)
		if err != nil {
			return nil, err
		}
		c.Data(http.StatusOK, "application/json; charset=utf-8", body)
		firstTokenMs = nil
	}

	return &ForwardResult{
		RequestID:        response.Header.Get("x-request-id"),
		Usage:            usage,
		Model:            parsed.Model,
		Stream:           parsed.Stream,
		Duration:         time.Since(startTime),
		FirstTokenMs:     firstTokenMs,
		ClientDisconnect: clientDisconnect,
	}, nil
}

func (s *GatewayService) forwardClaudeWebCountTokens(c *gin.Context, parsed *ParsedRequest) error {
	prompt, err := BuildClaudeWebPrompt(parsed.Body.Bytes())
	if err != nil {
		s.countTokensError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return err
	}
	inputTokens := estimateClaudeWebTokens(utf8.RuneCountInString(prompt))
	c.JSON(http.StatusOK, gin.H{"input_tokens": inputTokens})
	return nil
}

type claudeWebFlushWriter struct {
	writer gin.ResponseWriter
}

func (w claudeWebFlushWriter) Write(data []byte) (int, error) {
	written, err := w.writer.Write(data)
	if err == nil {
		w.writer.Flush()
	}
	return written, err
}

func claudeWebForwardError(err error) error {
	var upstreamErr *ClaudeWebHTTPError
	if !errors.As(err, &upstreamErr) {
		return err
	}
	kind := upstreamErr.Kind
	if kind == "" {
		kind = ClassifyClaudeWebResponse(upstreamErr.StatusCode, upstreamErr.Header, nil)
	}
	body, _ := json.Marshal(map[string]any{
		"type": "error",
		"error": map[string]string{
			"type":    string(kind),
			"message": claudeWebPublicErrorMessage(kind),
		},
	})
	return &UpstreamFailoverError{
		StatusCode:      upstreamErr.StatusCode,
		ResponseBody:    body,
		ResponseHeaders: upstreamErr.Header.Clone(),
	}
}

func claudeWebPublicErrorMessage(kind ClaudeWebErrorKind) string {
	switch kind {
	case ClaudeWebErrorExpired:
		return "Claude Web session expired; update the account Cookie"
	case ClaudeWebErrorCloudflare:
		return "Claude Web browser verification failed; check the account proxy and Cookie"
	case ClaudeWebErrorRateLimited:
		return "Claude Web account is rate limited"
	default:
		return "Claude Web upstream request failed"
	}
}

var _ io.Writer = claudeWebFlushWriter{}

func formatClaudeWebError(kind ClaudeWebErrorKind, status int) error {
	return fmt.Errorf("%s (status %d)", kind, status)
}
