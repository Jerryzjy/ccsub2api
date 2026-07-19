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
	ClaudeWebErrorExpired       ClaudeWebErrorKind = "web_session_expired"
	ClaudeWebErrorCloudflare    ClaudeWebErrorKind = "web_session_cloudflare"
	ClaudeWebErrorRegionBlocked ClaudeWebErrorKind = "web_session_region_blocked"
	ClaudeWebErrorRateLimited   ClaudeWebErrorKind = "web_session_rate_limited"
	ClaudeWebErrorUpstream      ClaudeWebErrorKind = "web_session_upstream"
	ClaudeWebErrorProxyRequired ClaudeWebErrorKind = "web_session_proxy_required"
)

// ClaudeWebStreamError represents an error event delivered inside a successful
// HTTP SSE response. It intentionally retains only the classified public kind,
// never the upstream body, because browser-session responses can contain
// sensitive account details.
type ClaudeWebStreamError struct {
	Kind ClaudeWebErrorKind
}

func (e *ClaudeWebStreamError) Error() string {
	if e == nil {
		return claudeWebPublicErrorMessage(ClaudeWebErrorUpstream)
	}
	return claudeWebPublicErrorMessage(e.Kind)
}

func ClassifyClaudeWebResponse(status int, header http.Header, body []byte) ClaudeWebErrorKind {
	switch status {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		if strings.Contains(strings.ToLower(header.Get("Location")), "app-unavailable-in-region") {
			return ClaudeWebErrorRegionBlocked
		}
		return ClaudeWebErrorUpstream
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
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	if err := s.enforceProxyRequirement(account, proxyURL); err != nil {
		writeClaudeWebLocalError(c, http.StatusBadGateway, string(ClaudeWebErrorProxyRequired), claudeWebPublicErrorMessage(ClaudeWebErrorProxyRequired))
		return nil, err
	}

	conversationCache := s.claudeWebConversationCache()
	conversationKey := s.claudeWebConversationKey(parsed, account)
	if conversationKey == "" {
		conversationCache = nil
	}
	unlock, err := acquireClaudeWebConversationLock(ctx, conversationCache, conversationKey)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		// Redis conversation state is an optimization. Fail open with the
		// original one-request conversation behavior when it is unavailable.
		conversationCache = nil
		conversationKey = ""
		unlock = func() {}
		claudeWebConversationMetrics.cacheUnavailable.Add(1)
	}
	defer unlock()

	state, loadErr := loadClaudeWebConversationState(ctx, conversationCache, conversationKey)
	if loadErr != nil {
		conversationCache = nil
		conversationKey = ""
		state = nil
		claudeWebConversationMetrics.cacheUnavailable.Add(1)
	}
	plan, err := PlanClaudeWebConversation(parsed, upstreamModel, state, time.Now())
	if err != nil {
		writeClaudeWebRequestError(c, err)
		return nil, err
	}
	recordClaudeWebConversationPlan(plan)
	payload, err := BuildClaudeWebCompletion(parsed.Body.Bytes(), upstreamModel)
	if err != nil {
		writeClaudeWebRequestError(c, err)
		return nil, err
	}
	payload.Prompt = plan.Prompt
	if plan.Reused {
		payload.ParentMessageUUID = plan.ParentMessageUUID
	}

	organizationID := ""
	conversationID := ""
	createdFresh := false
	if plan.Reused && state != nil {
		organizationID = state.OrganizationID
		conversationID = state.ConversationID
	} else {
		if state != nil && conversationCache != nil {
			_ = conversationCache.DeleteClaudeWebConversation(ctx, conversationKey)
			deleteClaudeWebConversationBestEffort(ctx, s.claudeWebClient, account, proxyURL, state.OrganizationID, state.ConversationID)
		}
		organizationID, err = s.claudeWebClient.ResolveOrganization(ctx, account, proxyURL)
		if err != nil {
			return nil, claudeWebForwardError(err)
		}
		conversationID, err = s.claudeWebClient.CreateConversation(ctx, account, proxyURL, organizationID)
		if err != nil {
			return nil, claudeWebForwardError(err)
		}
		createdFresh = true
	}

	retainConversation := conversationCache != nil && conversationKey != ""
	cleanupFreshConversation := func() {
		if createdFresh {
			deleteClaudeWebConversationBestEffort(ctx, s.claudeWebClient, account, proxyURL, organizationID, conversationID)
		}
	}

	response, err := s.claudeWebClient.Complete(ctx, account, proxyURL, organizationID, conversationID, payload)
	if err != nil {
		if plan.Reused && isClaudeWebConversationInvalid(err) {
			claudeWebConversationMetrics.rebuild.Add(1)
			_ = conversationCache.DeleteClaudeWebConversation(ctx, conversationKey)
			deleteClaudeWebConversationBestEffort(ctx, s.claudeWebClient, account, proxyURL, organizationID, conversationID)

			plan, err = PlanClaudeWebConversation(parsed, upstreamModel, nil, time.Now())
			if err != nil {
				return nil, err
			}
			payload, err = BuildClaudeWebCompletion(parsed.Body.Bytes(), upstreamModel)
			if err != nil {
				return nil, err
			}
			payload.Prompt = plan.Prompt
			organizationID, err = s.claudeWebClient.ResolveOrganization(ctx, account, proxyURL)
			if err != nil {
				return nil, claudeWebForwardError(err)
			}
			conversationID, err = s.claudeWebClient.CreateConversation(ctx, account, proxyURL, organizationID)
			if err != nil {
				return nil, claudeWebForwardError(err)
			}
			createdFresh = true
			response, err = s.claudeWebClient.Complete(ctx, account, proxyURL, organizationID, conversationID, payload)
		}
		if err != nil {
			cleanupFreshConversation()
			return nil, claudeWebForwardError(err)
		}
	}
	defer response.Body.Close()
	if parsed.OnUpstreamAccepted != nil {
		parsed.OnUpstreamAccepted()
	}

	messageID := "msg_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	initialUsage := ClaudeWebConversationUsage(plan, retainConversation)
	streamMeta := ClaudeWebStreamMeta{
		Model:                      parsed.Model,
		MessageID:                  messageID,
		InputTokens:                initialUsage.InputTokens,
		CacheCreationInputTokens:   initialUsage.CacheCreationInputTokens,
		CacheReadInputTokens:       initialUsage.CacheReadInputTokens,
		CacheCreation5mInputTokens: initialUsage.CacheCreation5mTokens,
		CacheCreation1hInputTokens: initialUsage.CacheCreation1hTokens,
	}
	var usage ClaudeUsage
	assistantDigest := ""
	firstTokenValue := int(time.Since(startTime).Milliseconds())
	firstTokenMs := &firstTokenValue
	clientDisconnect := false

	if parsed.Stream {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")
		usage, assistantDigest, err = TranslateClaudeWebSSEWithDigest(response.Body, claudeWebFlushWriter{writer: c.Writer}, streamMeta)
		if err != nil {
			if conversationCache != nil {
				_ = conversationCache.DeleteClaudeWebConversation(ctx, conversationKey)
			}
			deleteClaudeWebConversationBestEffort(ctx, s.claudeWebClient, account, proxyURL, organizationID, conversationID)
			if ctx.Err() != nil {
				clientDisconnect = true
			}
			return nil, claudeWebForwardError(err)
		}
	} else {
		var body []byte
		body, usage, assistantDigest, err = AggregateClaudeWebSSEWithDigestMeta(response.Body, streamMeta)
		if err != nil {
			if conversationCache != nil {
				_ = conversationCache.DeleteClaudeWebConversation(ctx, conversationKey)
			}
			deleteClaudeWebConversationBestEffort(ctx, s.claudeWebClient, account, proxyURL, organizationID, conversationID)
			return nil, claudeWebForwardError(err)
		}
		c.Data(http.StatusOK, "application/json; charset=utf-8", body)
		firstTokenMs = nil
	}

	if retainConversation {
		now := time.Now()
		createdAt := now
		contextTokens := plan.NewInputTokensEstimated + usage.OutputTokens
		if plan.Reused && state != nil {
			createdAt = state.CreatedAt
			if createdAt.IsZero() {
				createdAt = now
			}
			contextTokens += state.ContextTokensEstimated
		}
		storedDigestChain := plan.DigestChain
		if assistantDigest != "" {
			storedDigestChain += "-" + assistantDigest
		}
		newState := ClaudeWebConversationState{
			OrganizationID:         organizationID,
			ConversationID:         conversationID,
			ParentMessageUUID:      payload.TurnMessageUUIDs.AssistantMessageUUID,
			Model:                  upstreamModel,
			DigestChain:            storedDigestChain,
			ContextTokensEstimated: contextTokens,
			CreatedAt:              createdAt,
			LastUsedAt:             now,
			TTLSeconds:             int(plan.TTL.Seconds()),
		}
		saveCtx, cancelSave := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		saveErr := saveClaudeWebConversationState(saveCtx, conversationCache, conversationKey, newState, plan.TTL)
		cancelSave()
		if saveErr != nil {
			deleteCtx, cancelDelete := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
			_ = conversationCache.DeleteClaudeWebConversation(deleteCtx, conversationKey)
			cancelDelete()
			deleteClaudeWebConversationBestEffort(ctx, s.claudeWebClient, account, proxyURL, organizationID, conversationID)
		}
	} else {
		cleanupFreshConversation()
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

func deleteClaudeWebConversationBestEffort(ctx context.Context, client *ClaudeWebClient, account *Account, proxyURL, organizationID, conversationID string) {
	if client == nil || account == nil || strings.TrimSpace(organizationID) == "" || strings.TrimSpace(conversationID) == "" {
		return
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	_ = client.DeleteConversation(cleanupCtx, account, proxyURL, organizationID, conversationID)
}

func isClaudeWebConversationInvalid(err error) bool {
	var upstreamErr *ClaudeWebHTTPError
	if !errors.As(err, &upstreamErr) {
		return false
	}
	return upstreamErr.StatusCode == http.StatusNotFound || upstreamErr.StatusCode == http.StatusGone
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

func writeClaudeWebRequestError(c *gin.Context, err error) {
	message := "Invalid Claude Web client request"
	if errors.Is(err, ErrClaudeWebUnsupportedContent) {
		message = "Claude Web sessions support text-only conversation history; use an Anthropic OAuth account for tool, image, or document blocks"
	}
	writeClaudeWebLocalError(c, http.StatusBadRequest, "invalid_request_error", message)
}

func writeClaudeWebLocalError(c *gin.Context, status int, errorType, message string) {
	if c == nil || c.Writer == nil || c.Writer.Written() {
		return
	}
	c.JSON(status, gin.H{
		"type": "error",
		"error": gin.H{
			"type":    errorType,
			"message": message,
		},
	})
	MarkResponseCommitted(c)
}

func claudeWebForwardError(err error) error {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var failoverErr *UpstreamFailoverError
	if errors.As(err, &failoverErr) {
		return err
	}
	var upstreamErr *ClaudeWebHTTPError
	if errors.As(err, &upstreamErr) {
		kind := upstreamErr.Kind
		if kind == "" {
			kind = ClassifyClaudeWebResponse(upstreamErr.StatusCode, upstreamErr.Header, nil)
		}
		return newClaudeWebFailoverError(upstreamErr.StatusCode, kind, upstreamErr.Header)
	}
	var streamErr *ClaudeWebStreamError
	if errors.As(err, &streamErr) {
		kind := streamErr.Kind
		if kind == "" {
			kind = ClaudeWebErrorUpstream
		}
		return newClaudeWebFailoverError(claudeWebStatusForKind(kind), kind, nil)
	}
	// Transport, response-decoding, and malformed-stream failures are upstream
	// account failures too. Normalize them so the handler can fail over to a
	// healthy account instead of returning a generic local 502 immediately.
	return newClaudeWebFailoverError(http.StatusBadGateway, ClaudeWebErrorUpstream, nil)
}

func newClaudeWebFailoverError(status int, kind ClaudeWebErrorKind, header http.Header) error {
	body, _ := json.Marshal(map[string]any{
		"type": "error",
		"error": map[string]string{
			"type":    string(kind),
			"message": claudeWebPublicErrorMessage(kind),
		},
	})
	return &UpstreamFailoverError{
		StatusCode:      status,
		ResponseBody:    body,
		ResponseHeaders: header.Clone(),
	}
}

func claudeWebStatusForKind(kind ClaudeWebErrorKind) int {
	switch kind {
	case ClaudeWebErrorExpired:
		return http.StatusUnauthorized
	case ClaudeWebErrorCloudflare:
		return http.StatusForbidden
	case ClaudeWebErrorRateLimited:
		return http.StatusTooManyRequests
	default:
		return http.StatusBadGateway
	}
}

// ClaudeWebPublicErrorFromBody recognizes only errors produced by the Claude
// Web adapter and regenerates the message from an allowlist. The upstream
// message is never trusted or exposed.
func ClaudeWebPublicErrorFromBody(body []byte) (ClaudeWebErrorKind, string, bool) {
	var envelope struct {
		Error struct {
			Type string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return "", "", false
	}
	kind := ClaudeWebErrorKind(strings.TrimSpace(envelope.Error.Type))
	switch kind {
	case ClaudeWebErrorExpired, ClaudeWebErrorCloudflare, ClaudeWebErrorRegionBlocked,
		ClaudeWebErrorRateLimited, ClaudeWebErrorUpstream, ClaudeWebErrorProxyRequired:
		return kind, claudeWebPublicErrorMessage(kind), true
	default:
		return "", "", false
	}
}

func claudeWebPublicErrorMessage(kind ClaudeWebErrorKind) string {
	switch kind {
	case ClaudeWebErrorExpired:
		return "Claude Web session expired; update the account Cookie"
	case ClaudeWebErrorCloudflare:
		return "Claude Web browser verification failed; check the account proxy and Cookie"
	case ClaudeWebErrorRegionBlocked:
		return "Claude Web is unavailable from the current outbound region; bind or replace the account proxy"
	case ClaudeWebErrorRateLimited:
		return "Claude Web account is rate limited"
	case ClaudeWebErrorProxyRequired:
		return "Claude Web account requires an upstream proxy; bind a proxy to the account or disable proxy.require_for_upstream"
	default:
		return "Claude Web upstream request failed"
	}
}

var _ io.Writer = claudeWebFlushWriter{}

func formatClaudeWebError(kind ClaudeWebErrorKind, status int) error {
	return fmt.Errorf("%s (status %d)", kind, status)
}
