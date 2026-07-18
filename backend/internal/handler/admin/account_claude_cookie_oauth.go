package admin

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// CreateClaudeCookieOAuthRequest creates an Anthropic OAuth account from an
// administrator-owned Claude Cookie or raw sessionKey.
type CreateClaudeCookieOAuthRequest struct {
	Name                    string         `json:"name" binding:"required"`
	Cookie                  string         `json:"cookie"`
	SessionKey              string         `json:"session_key"`
	Notes                   *string        `json:"notes"`
	Extra                   map[string]any `json:"extra"`
	ProxyID                 *int64         `json:"proxy_id"`
	Concurrency             int            `json:"concurrency"`
	Priority                int            `json:"priority"`
	RateMultiplier          *float64       `json:"rate_multiplier"`
	LoadFactor              *int           `json:"load_factor"`
	GroupIDs                []int64        `json:"group_ids"`
	ConfirmMixedChannelRisk *bool          `json:"confirm_mixed_channel_risk"`
}

// CreateClaudeCookieOAuth exchanges the submitted web session for OAuth
// credentials and returns only the redacted account response.
func (h *AccountHandler) CreateClaudeCookieOAuth(c *gin.Context) {
	var req CreateClaudeCookieOAuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if h.oauthService == nil {
		response.BadRequest(c, "Claude OAuth service is unavailable")
		return
	}
	if req.RateMultiplier != nil && *req.RateMultiplier < 0 {
		response.BadRequest(c, "rate_multiplier must be >= 0")
		return
	}

	sessionKey := strings.TrimSpace(req.SessionKey)
	if strings.TrimSpace(req.Cookie) != "" {
		normalized, err := service.NormalizeClaudeWebCookie(req.Cookie, time.Now())
		if err != nil {
			response.BadRequest(c, "Invalid Claude Cookie")
			return
		}
		if normalized.SessionKey != "" {
			sessionKey = normalized.SessionKey
		}
	}
	if sessionKey == "" {
		response.BadRequest(c, "Cookie or sessionKey is required")
		return
	}

	tokenInfo, err := h.oauthService.CookieAuth(c.Request.Context(), &service.CookieAuthInput{
		SessionKey: sessionKey,
		ProxyID:    req.ProxyID,
		Scope:      service.CookieAuthScopeClaudeAI,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if strings.TrimSpace(tokenInfo.AccessToken) == "" {
		response.BadRequest(c, "Claude OAuth response did not include an access token")
		return
	}
	if strings.TrimSpace(tokenInfo.RefreshToken) == "" {
		response.BadRequest(c, "Claude OAuth response did not include a refresh token")
		return
	}

	credentials := map[string]any{
		"access_token":  tokenInfo.AccessToken,
		"refresh_token": tokenInfo.RefreshToken,
		"expires_at":    strconv.FormatInt(tokenInfo.ExpiresAt, 10),
		"scope":         tokenInfo.Scope,
		"token_type":    tokenInfo.TokenType,
	}
	extra := req.Extra
	if extra == nil {
		extra = make(map[string]any)
	}
	if tokenInfo.OrgUUID != "" {
		extra["org_uuid"] = tokenInfo.OrgUUID
	}
	if tokenInfo.AccountUUID != "" {
		extra["account_uuid"] = tokenInfo.AccountUUID
	}
	if tokenInfo.EmailAddress != "" {
		extra["email_address"] = tokenInfo.EmailAddress
	}

	skipMixedChannelCheck := req.ConfirmMixedChannelRisk != nil && *req.ConfirmMixedChannelRisk
	account, err := h.adminService.CreateAccount(c.Request.Context(), &service.CreateAccountInput{
		Name:                  req.Name,
		Notes:                 req.Notes,
		Platform:              service.PlatformAnthropic,
		Type:                  service.AccountTypeOAuth,
		Credentials:           credentials,
		Extra:                 extra,
		ProxyID:               req.ProxyID,
		Concurrency:           req.Concurrency,
		Priority:              req.Priority,
		RateMultiplier:        req.RateMultiplier,
		LoadFactor:            req.LoadFactor,
		GroupIDs:              req.GroupIDs,
		SkipMixedChannelCheck: skipMixedChannelCheck,
	})
	if err != nil {
		var mixedErr *service.MixedChannelError
		if errors.As(err, &mixedErr) {
			c.JSON(409, gin.H{"error": "mixed_channel_warning", "message": mixedErr.Error()})
			return
		}
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, h.buildAccountResponseWithRuntime(c.Request.Context(), account))
}
