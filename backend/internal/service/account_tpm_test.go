package service

import (
	"encoding/json"
	"testing"
)

func TestGetBaseTPM(t *testing.T) {
	tests := []struct {
		name     string
		extra    map[string]any
		expected int
	}{
		{"nil extra", nil, 0},
		{"no key", map[string]any{}, 0},
		{"zero", map[string]any{"base_tpm": 0}, 0},
		{"int value", map[string]any{"base_tpm": 200000}, 200000},
		{"float value", map[string]any{"base_tpm": 200000.0}, 200000},
		{"string value", map[string]any{"base_tpm": "200000"}, 200000},
		{"negative value", map[string]any{"base_tpm": -5}, 0},
		{"int64 value", map[string]any{"base_tpm": int64(500000)}, 500000},
		{"json.Number value", map[string]any{"base_tpm": json.Number("250000")}, 250000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Account{Extra: tt.extra}
			if got := a.GetBaseTPM(); got != tt.expected {
				t.Errorf("GetBaseTPM() = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestGetTPMStrategy(t *testing.T) {
	tests := []struct {
		name     string
		extra    map[string]any
		expected string
	}{
		{"nil extra", nil, "tiered"},
		{"no key", map[string]any{}, "tiered"},
		{"tiered", map[string]any{"tpm_strategy": "tiered"}, "tiered"},
		{"sticky_exempt", map[string]any{"tpm_strategy": "sticky_exempt"}, "sticky_exempt"},
		{"invalid", map[string]any{"tpm_strategy": "foobar"}, "tiered"},
		{"empty string fallback", map[string]any{"tpm_strategy": ""}, "tiered"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Account{Extra: tt.extra}
			if got := a.GetTPMStrategy(); got != tt.expected {
				t.Errorf("GetTPMStrategy() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestGetTPMStickyBuffer(t *testing.T) {
	tests := []struct {
		name     string
		extra    map[string]any
		expected int
	}{
		{"nil extra", nil, 0},
		{"base_tpm=0", map[string]any{"base_tpm": 0}, 0},
		// 默认 = base/5
		{"base=200000 → 40000", map[string]any{"base_tpm": 200000}, 40000},
		{"base=10 → floor 2", map[string]any{"base_tpm": 10}, 2},
		{"base=1 → floor 1", map[string]any{"base_tpm": 1}, 1},
		// 手动 override
		{"custom buffer", map[string]any{"base_tpm": 200000, "tpm_sticky_buffer": 5000}, 5000},
		{"custom buffer=0 fallback", map[string]any{"base_tpm": 200000, "tpm_sticky_buffer": 0}, 40000},
		{"custom buffer negative fallback", map[string]any{"base_tpm": 200000, "tpm_sticky_buffer": -1}, 40000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Account{Extra: tt.extra}
			if got := a.GetTPMStickyBuffer(); got != tt.expected {
				t.Errorf("GetTPMStickyBuffer() = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestCheckTPMSchedulability(t *testing.T) {
	tests := []struct {
		name       string
		extra      map[string]any
		currentTPM int
		expected   WindowCostSchedulability
	}{
		{"disabled", map[string]any{}, 1_000_000, WindowCostSchedulable},
		{"green zone", map[string]any{"base_tpm": 200000}, 100000, WindowCostSchedulable},
		{"yellow zone tiered (at limit)", map[string]any{"base_tpm": 200000}, 200000, WindowCostStickyOnly},
		{"yellow zone tiered (within buffer)", map[string]any{"base_tpm": 200000}, 220000, WindowCostStickyOnly},
		{"red zone tiered (over buffer)", map[string]any{"base_tpm": 200000}, 260000, WindowCostNotSchedulable},
		{"sticky_exempt at limit", map[string]any{"base_tpm": 200000, "tpm_strategy": "sticky_exempt"}, 200000, WindowCostStickyOnly},
		{"sticky_exempt far over", map[string]any{"base_tpm": 200000, "tpm_strategy": "sticky_exempt"}, 5_000_000, WindowCostStickyOnly},
		{"custom buffer yellow", map[string]any{"base_tpm": 100000, "tpm_sticky_buffer": 50000}, 140000, WindowCostStickyOnly},
		{"custom buffer red", map[string]any{"base_tpm": 100000, "tpm_sticky_buffer": 50000}, 150000, WindowCostNotSchedulable},
		{"negative currentTPM", map[string]any{"base_tpm": 200000}, -1, WindowCostSchedulable},
		{"base_tpm negative disabled", map[string]any{"base_tpm": -5}, 100000, WindowCostSchedulable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Account{Extra: tt.extra}
			if got := a.CheckTPMSchedulability(tt.currentTPM); got != tt.expected {
				t.Errorf("CheckTPMSchedulability(%d) = %d, want %d", tt.currentTPM, got, tt.expected)
			}
		})
	}
}
