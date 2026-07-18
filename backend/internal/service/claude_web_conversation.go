package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	claudeWebConversationDefaultTTL = 5 * time.Minute
	claudeWebConversationMaxTTL     = time.Hour
	claudeWebConversationLockTTL    = 15 * time.Minute
	claudeWebConversationLockWait   = 60 * time.Second
)

var errClaudeWebConversationLockTimeout = errors.New("Claude Web conversation is busy")

// ClaudeWebConversationCache is an optional extension implemented by the
// Redis-backed gateway cache. Keeping it separate from GatewayCache avoids
// forcing lightweight scheduler/test caches to store browser conversation
// state.
type ClaudeWebConversationCache interface {
	GetClaudeWebConversation(ctx context.Context, key string) ([]byte, error)
	SetClaudeWebConversation(ctx context.Context, key string, value []byte, ttl time.Duration) error
	DeleteClaudeWebConversation(ctx context.Context, key string) error
	TryLockClaudeWebConversation(ctx context.Context, key, owner string, ttl time.Duration) (bool, error)
	UnlockClaudeWebConversation(ctx context.Context, key, owner string) error
}

type ClaudeWebConversationState struct {
	OrganizationID         string    `json:"organization_id"`
	ConversationID         string    `json:"conversation_id"`
	ParentMessageUUID      string    `json:"parent_message_uuid"`
	Model                  string    `json:"model"`
	DigestChain            string    `json:"digest_chain"`
	ContextTokensEstimated int       `json:"context_tokens_estimated"`
	CreatedAt              time.Time `json:"created_at"`
	LastUsedAt             time.Time `json:"last_used_at"`
	TTLSeconds             int       `json:"ttl_seconds"`
}

type ClaudeWebConversationPlan struct {
	Prompt                     string
	ParentMessageUUID          string
	DigestChain                string
	Reused                     bool
	MissReason                 string
	TTL                        time.Duration
	ReusedInputTokensEstimated int
	NewInputTokensEstimated    int
}

// ClaudeWebConversationUsage maps locally observed conversation reuse into the
// standard Anthropic usage buckets. Claude Web does not return API prompt-cache
// counters, so these values describe Sub2API's retained Web conversation:
// newly retained context is a cache creation and previously retained context is
// a cache read. When state cannot be retained, input remains ordinary input.
func ClaudeWebConversationUsage(plan ClaudeWebConversationPlan, retained bool) ClaudeUsage {
	if !retained {
		return ClaudeUsage{InputTokens: plan.NewInputTokensEstimated}
	}

	usage := ClaudeUsage{
		CacheCreationInputTokens: plan.NewInputTokensEstimated,
	}
	if plan.Reused {
		usage.CacheReadInputTokens = plan.ReusedInputTokensEstimated
	}
	if plan.TTL >= claudeWebConversationMaxTTL {
		usage.CacheCreation1hTokens = plan.NewInputTokensEstimated
	} else {
		usage.CacheCreation5mTokens = plan.NewInputTokensEstimated
	}
	return usage
}

// PlanClaudeWebConversation decides whether a request is a strict extension
// of the conversation already stored upstream. A hit sends only the latest
// user turn; a miss sends the complete flattened prompt and starts a fresh
// Claude Web conversation.
func PlanClaudeWebConversation(parsed *ParsedRequest, model string, state *ClaudeWebConversationState, now time.Time) (ClaudeWebConversationPlan, error) {
	fullPrompt, err := BuildClaudeWebPrompt(parsed.Body.Bytes())
	if err != nil {
		return ClaudeWebConversationPlan{}, err
	}
	digestChain, err := buildClaudeWebDigestChain(parsed)
	if err != nil {
		return ClaudeWebConversationPlan{}, err
	}

	plan := ClaudeWebConversationPlan{
		Prompt:      fullPrompt,
		DigestChain: digestChain,
		TTL:         ClaudeWebConversationTTL(parsed),
		MissReason:  "first_turn",
	}
	plan.NewInputTokensEstimated = estimateClaudeWebTokens(len([]rune(plan.Prompt)))

	if state == nil {
		return plan, nil
	}
	if strings.TrimSpace(state.ConversationID) == "" || strings.TrimSpace(state.ParentMessageUUID) == "" {
		plan.MissReason = "invalid_state"
		return plan, nil
	}
	if state.Model != model {
		plan.MissReason = "model_changed"
		return plan, nil
	}
	stateTTL := time.Duration(state.TTLSeconds) * time.Second
	if stateTTL <= 0 || stateTTL > claudeWebConversationMaxTTL {
		stateTTL = claudeWebConversationDefaultTTL
	}
	if state.LastUsedAt.IsZero() || !now.Before(state.LastUsedAt.Add(stateTTL)) {
		plan.MissReason = "ttl_expired"
		return plan, nil
	}
	if state.DigestChain == "" || plan.DigestChain == state.DigestChain ||
		!strings.HasPrefix(plan.DigestChain, state.DigestChain+"-") {
		plan.MissReason = "history_diverged"
		return plan, nil
	}

	latestPrompt, err := latestClaudeWebUserPrompt(parsed)
	if err != nil {
		plan.MissReason = "unsupported_follow_up"
		return plan, nil
	}

	plan.Prompt = latestPrompt
	plan.ParentMessageUUID = state.ParentMessageUUID
	plan.Reused = true
	plan.MissReason = ""
	plan.ReusedInputTokensEstimated = state.ContextTokensEstimated
	plan.NewInputTokensEstimated = estimateClaudeWebTokens(len([]rune(latestPrompt)))
	return plan, nil
}

// buildClaudeWebDigestChain hashes the text semantics that are actually sent
// to Claude Web. Anthropic content strings and single text-block arrays are
// equivalent for this adapter, and cache_control metadata does not change the
// flattened upstream prompt, so neither difference should invalidate reuse.
func buildClaudeWebDigestChain(parsed *ParsedRequest) (string, error) {
	if parsed == nil || parsed.Body == nil {
		return "", errors.New("empty request")
	}
	decoder := json.NewDecoder(bytes.NewReader(parsed.Body.Bytes()))
	decoder.UseNumber()
	var request claudeWebAnthropicRequest
	if err := decoder.Decode(&request); err != nil {
		return "", fmt.Errorf("decode anthropic request: %w", err)
	}

	parts := make([]string, 0, len(request.Messages)+1)
	if rawSystem := bytes.TrimSpace(request.System); len(rawSystem) > 0 && !bytes.Equal(rawSystem, []byte("null")) {
		text, err := claudeWebContentText(request.System)
		if err != nil {
			return "", fmt.Errorf("system: %w", err)
		}
		parts = append(parts, claudeWebTextDigest("s", text))
	}
	for i, message := range request.Messages {
		text, err := claudeWebContentText(message.Content)
		if err != nil {
			return "", fmt.Errorf("messages[%d]: %w", i, err)
		}
		parts = append(parts, claudeWebTextDigest(rolePrefix(message.Role), text))
	}
	return strings.Join(parts, "-"), nil
}

func ClaudeWebConversationTTL(parsed *ParsedRequest) time.Duration {
	if parsed == nil || parsed.Body == nil {
		return claudeWebConversationDefaultTTL
	}
	decoder := json.NewDecoder(bytes.NewReader(parsed.Body.Bytes()))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err == nil && claudeWebContainsOneHourTTL(value) {
		return claudeWebConversationMaxTTL
	}
	return claudeWebConversationDefaultTTL
}

func claudeWebContainsOneHourTTL(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		if ttl, ok := typed["ttl"].(string); ok && strings.EqualFold(strings.TrimSpace(ttl), "1h") {
			return true
		}
		for _, child := range typed {
			if claudeWebContainsOneHourTTL(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if claudeWebContainsOneHourTTL(child) {
				return true
			}
		}
	}
	return false
}

func latestClaudeWebUserPrompt(parsed *ParsedRequest) (string, error) {
	if parsed == nil {
		return "", errors.New("empty request")
	}
	var messages []claudeWebAnthropicMessage
	if raw := parsed.MessagesRaw(); len(raw) == 0 {
		return "", errors.New("messages is required")
	} else if err := json.Unmarshal(raw, &messages); err != nil {
		return "", errors.New("invalid messages")
	}
	if len(messages) == 0 || messages[len(messages)-1].Role != "user" {
		return "", errors.New("latest message must be user")
	}
	prompt, err := claudeWebContentText(messages[len(messages)-1].Content)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(prompt) == "" {
		return "", errors.New("latest user message is empty")
	}
	return prompt, nil
}

func (s *GatewayService) claudeWebConversationKey(parsed *ParsedRequest, account *Account) string {
	if s == nil || parsed == nil || parsed.SessionContext == nil || parsed.SessionContext.APIKeyID <= 0 || account == nil || account.ID <= 0 {
		return ""
	}
	credentialFingerprint := claudeWebCredentialFingerprint(account)
	if credentialFingerprint == "" {
		return ""
	}
	sessionHash := s.GenerateSessionHash(parsed)
	if strings.TrimSpace(sessionHash) == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(sessionHash))
	return fmt.Sprintf("%d:%d:%d:%s:%s",
		parsed.SessionContext.APIKeyID,
		derefGroupID(parsed.GroupID),
		account.ID,
		credentialFingerprint,
		hex.EncodeToString(digest[:16]),
	)
}

func claudeWebCredentialFingerprint(account *Account) string {
	if account == nil {
		return ""
	}
	sessionKey := strings.TrimSpace(account.GetCredential(ClaudeWebCredentialSessionKey))
	if rawCookie := strings.TrimSpace(account.GetCredential(ClaudeWebCredentialCookie)); rawCookie != "" {
		if normalized, err := NormalizeClaudeWebCookie(rawCookie, time.Now()); err == nil && normalized.SessionKey != "" {
			sessionKey = normalized.SessionKey
		}
	}
	if sessionKey == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(sessionKey))
	return hex.EncodeToString(digest[:16])
}

func (s *GatewayService) claudeWebConversationCache() ClaudeWebConversationCache {
	if s == nil || s.cache == nil {
		return nil
	}
	cache, _ := s.cache.(ClaudeWebConversationCache)
	return cache
}

func loadClaudeWebConversationState(ctx context.Context, cache ClaudeWebConversationCache, key string) (*ClaudeWebConversationState, error) {
	if cache == nil || key == "" {
		return nil, nil
	}
	raw, err := cache.GetClaudeWebConversation(ctx, key)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, nil
	}
	var state ClaudeWebConversationState
	if err := json.Unmarshal(raw, &state); err != nil {
		_ = cache.DeleteClaudeWebConversation(ctx, key)
		return nil, nil
	}
	return &state, nil
}

func saveClaudeWebConversationState(ctx context.Context, cache ClaudeWebConversationCache, key string, state ClaudeWebConversationState, ttl time.Duration) error {
	if cache == nil || key == "" {
		return nil
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return cache.SetClaudeWebConversation(ctx, key, raw, ttl)
}

func acquireClaudeWebConversationLock(ctx context.Context, cache ClaudeWebConversationCache, key string) (func(), error) {
	if cache == nil || key == "" {
		return func() {}, nil
	}
	owner := uuid.NewString()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	timer := time.NewTimer(claudeWebConversationLockWait)
	defer timer.Stop()
	for {
		acquired, err := cache.TryLockClaudeWebConversation(ctx, key, owner, claudeWebConversationLockTTL)
		if err != nil {
			return nil, err
		}
		if acquired {
			return func() {
				unlockCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
				defer cancel()
				_ = cache.UnlockClaudeWebConversation(unlockCtx, key, owner)
			}, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
			return nil, errClaudeWebConversationLockTimeout
		case <-ticker.C:
		}
	}
}
