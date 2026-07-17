package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

const ClaudeWebSafetyPresetVersion = 1

type ClaudeWebSafetyPreset struct {
	BaseRPM                 int
	BaseTPM                 int
	MaxSessions             int
	WindowCostLimit         float64
	WindowCostStickyReserve float64
}

var claudeWebSafetyPresets = map[string]ClaudeWebSafetyPreset{
	ClaudeTierPro:    {BaseRPM: 6, BaseTPM: 80000, MaxSessions: 2, WindowCostLimit: 8, WindowCostStickyReserve: 1},
	ClaudeTierMax5x:  {BaseRPM: 12, BaseTPM: 160000, MaxSessions: 4, WindowCostLimit: 40, WindowCostStickyReserve: 5},
	ClaudeTierMax20x: {BaseRPM: 20, BaseTPM: 240000, MaxSessions: 6, WindowCostLimit: 80, WindowCostStickyReserve: 10},
}

var claudeWebManagedUsageKeys = map[string]struct{}{
	"quota_used": {}, "quota_daily_used": {}, "quota_weekly_used": {},
	"quota_daily_start": {}, "quota_weekly_start": {},
	"quota_daily_reset_at": {}, "quota_weekly_reset_at": {},
}

func ClaudeWebSafetyPresetForTier(tier string) (ClaudeWebSafetyPreset, bool) {
	preset, ok := claudeWebSafetyPresets[NormalizeClaudeTier(tier)]
	return preset, ok
}

func NormalizeClaudeWebSafetyExtra(extra map[string]any, applyDefaults bool) (map[string]any, error) {
	normalized := make(map[string]any, len(extra)+10)
	for key, value := range extra {
		if _, managed := claudeWebManagedUsageKeys[key]; !managed {
			normalized[key] = value
		}
	}

	tier := ClaudeTierPro
	tierExplicit := false
	if raw, exists := normalized["claude_tier"]; exists {
		tierExplicit = true
		value, ok := raw.(string)
		if !ok || NormalizeClaudeTier(value) == "" {
			return nil, errors.New("invalid Claude tier")
		}
		tier = NormalizeClaudeTier(value)
	}
	normalized["claude_tier"] = tier

	if raw, exists := normalized["claude_tier_source"]; exists {
		source, ok := raw.(string)
		if !ok || !isClaudeTierSource(source) {
			return nil, errors.New("invalid Claude tier source")
		}
		normalized["claude_tier_source"] = strings.TrimSpace(source)
	} else {
		if tierExplicit {
			normalized["claude_tier_source"] = ClaudeTierSourceManual
		} else {
			normalized["claude_tier_source"] = ClaudeTierSourceInferred
		}
	}

	positiveKeys := []string{
		"base_rpm", "base_tpm", "max_sessions", "session_idle_timeout_minutes",
		"window_cost_limit", "window_cost_sticky_reserve", "quota_limit",
		"quota_daily_limit", "quota_weekly_limit",
	}
	for _, key := range positiveKeys {
		if raw, exists := normalized[key]; exists {
			value, ok := claudeWebPositiveNumber(raw)
			if !ok {
				return nil, fmt.Errorf("%s must be a positive finite number", key)
			}
			normalized[key] = value
		}
	}
	for _, key := range []string{"rpm_strategy", "tpm_strategy"} {
		if raw, exists := normalized[key]; exists {
			value, ok := raw.(string)
			if !ok || (value != "tiered" && value != "sticky_exempt") {
				return nil, fmt.Errorf("invalid %s", key)
			}
		}
	}

	if applyDefaults {
		preset := claudeWebSafetyPresets[tier]
		setClaudeWebDefault(normalized, "base_rpm", preset.BaseRPM)
		setClaudeWebDefault(normalized, "base_tpm", preset.BaseTPM)
		setClaudeWebDefault(normalized, "max_sessions", preset.MaxSessions)
		setClaudeWebDefault(normalized, "session_idle_timeout_minutes", 5)
		setClaudeWebDefault(normalized, "window_cost_limit", preset.WindowCostLimit)
		setClaudeWebDefault(normalized, "window_cost_sticky_reserve", preset.WindowCostStickyReserve)
		setClaudeWebDefault(normalized, "rpm_strategy", "tiered")
		setClaudeWebDefault(normalized, "tpm_strategy", "tiered")
		normalized["claude_safety_preset_version"] = ClaudeWebSafetyPresetVersion
	}
	return normalized, nil
}

func setClaudeWebDefault(extra map[string]any, key string, value any) {
	if _, exists := extra[key]; !exists {
		extra[key] = value
	}
}

func isClaudeTierSource(value string) bool {
	switch strings.TrimSpace(value) {
	case ClaudeTierSourceManual, ClaudeTierSourceProfile, ClaudeTierSourceInferred, ClaudeTierSourceProfileDefault:
		return true
	default:
		return false
	}
}

func claudeWebPositiveNumber(value any) (any, bool) {
	var number float64
	switch v := value.(type) {
	case int:
		number = float64(v)
	case int32:
		number = float64(v)
	case int64:
		number = float64(v)
	case float32:
		number = float64(v)
	case float64:
		number = v
	case json.Number:
		var err error
		number, err = v.Float64()
		if err != nil {
			return nil, false
		}
	case string:
		var err error
		number, err = strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return nil, false
		}
	default:
		return nil, false
	}
	if math.IsNaN(number) || math.IsInf(number, 0) || number <= 0 {
		return nil, false
	}
	if math.Trunc(number) == number {
		return int(number), true
	}
	return number, true
}
