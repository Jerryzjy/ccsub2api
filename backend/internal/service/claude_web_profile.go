package service

import "strings"

const (
	ClaudeTierPro    = "Pro"
	ClaudeTierMax5x  = "Max_5x"
	ClaudeTierMax20x = "Max_20x"

	ClaudeTierSourceManual         = "manual"
	ClaudeTierSourceProfile        = "profile"
	ClaudeTierSourceInferred       = "inferred"
	ClaudeTierSourceProfileDefault = "profile_default"
)

type ClaudeWebProfile struct {
	EmailAddress       string
	Tier               string
	TierSource         string
	SubscriptionActive bool
}

type claudeWebAccountProfile struct {
	EmailAddress string                `json:"email_address"`
	Memberships  []claudeWebMembership `json:"memberships"`
}

type claudeWebMembership struct {
	SeatTier     string                       `json:"seat_tier"`
	Organization claudeWebProfileOrganization `json:"organization"`
}

type claudeWebProfileOrganization struct {
	BillingType   string `json:"billing_type"`
	RateLimitTier string `json:"rate_limit_tier"`
}

func NormalizeClaudeTier(value string) string {
	normalized := strings.NewReplacer("-", "", "_", "", " ", "").Replace(strings.ToLower(strings.TrimSpace(value)))
	switch normalized {
	case "pro", "claudepro", "proplan":
		return ClaudeTierPro
	case "max5x", "claudemax5x", "max5":
		return ClaudeTierMax5x
	case "max20x", "claudemax20x", "max20":
		return ClaudeTierMax20x
	default:
		return ""
	}
}

func normalizeClaudeWebProfile(profile claudeWebAccountProfile) ClaudeWebProfile {
	result := ClaudeWebProfile{EmailAddress: strings.TrimSpace(profile.EmailAddress)}
	for _, membership := range profile.Memberships {
		if tier := NormalizeClaudeTier(membership.SeatTier); tier != "" {
			result.Tier = tier
			result.TierSource = ClaudeTierSourceProfile
			result.SubscriptionActive = true
			return result
		}
		if tier := NormalizeClaudeTier(membership.Organization.RateLimitTier); tier != "" {
			result.Tier = tier
			result.TierSource = ClaudeTierSourceProfile
			result.SubscriptionActive = true
			return result
		}
		if isClaudeWebPaidBillingType(membership.Organization.BillingType) {
			result.SubscriptionActive = true
		}
	}
	if result.SubscriptionActive {
		result.Tier = ClaudeTierPro
		result.TierSource = ClaudeTierSourceProfileDefault
	}
	return result
}

func isClaudeWebPaidBillingType(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" || normalized == "free" || normalized == "none" {
		return false
	}
	return strings.Contains(normalized, "subscription") || strings.Contains(normalized, "paid") || strings.Contains(normalized, "stripe")
}
