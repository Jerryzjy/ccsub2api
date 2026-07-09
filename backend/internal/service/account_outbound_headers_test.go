package service

import (
	"net/http"
	"testing"
)

func TestGetOutboundHeaderOverrides(t *testing.T) {
	tests := []struct {
		name     string
		extra    map[string]any
		expected map[string]string
	}{
		{"nil extra", nil, nil},
		{"no key", map[string]any{}, nil},
		{"basic override", map[string]any{"outbound_header_overrides": map[string]any{"X-Pool-Key": "abc"}}, map[string]string{"X-Pool-Key": "abc"}},
		{"drops forbidden hop-by-hop", map[string]any{"outbound_header_overrides": map[string]any{"Host": "evil", "X-Ok": "1"}}, map[string]string{"X-Ok": "1"}},
		{"drops protected authorization", map[string]any{"outbound_header_overrides": map[string]any{"authorization": "Bearer x", "X-Ok": "1"}}, map[string]string{"X-Ok": "1"}},
		{"drops protected x-api-key (any case)", map[string]any{"outbound_header_overrides": map[string]any{"X-Api-Key": "k", "X-Ok": "1"}}, map[string]string{"X-Ok": "1"}},
		{"non-string value ignored", map[string]any{"outbound_header_overrides": map[string]any{"X-Num": 5, "X-Ok": "1"}}, map[string]string{"X-Ok": "1"}},
		{"all dropped → nil", map[string]any{"outbound_header_overrides": map[string]any{"Host": "x"}}, nil},
		{"wrong type → nil", map[string]any{"outbound_header_overrides": "notamap"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Account{Extra: tt.extra}
			got := a.GetOutboundHeaderOverrides()
			if len(got) != len(tt.expected) {
				t.Fatalf("len=%d want %d (%v)", len(got), len(tt.expected), got)
			}
			for k, v := range tt.expected {
				if got[k] != v {
					t.Errorf("key %q = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestGetOutboundHeaderRemoves(t *testing.T) {
	tests := []struct {
		name     string
		extra    map[string]any
		expected []string
	}{
		{"nil extra", nil, nil},
		{"basic", map[string]any{"outbound_header_removes": []any{"X-Foo", "X-Bar"}}, []string{"X-Foo", "X-Bar"}},
		{"drops protected + forbidden", map[string]any{"outbound_header_removes": []any{"authorization", "connection", "X-Keep"}}, []string{"X-Keep"}},
		{"non-string ignored", map[string]any{"outbound_header_removes": []any{5, "X-Keep"}}, []string{"X-Keep"}},
		{"all dropped → nil", map[string]any{"outbound_header_removes": []any{"authorization"}}, nil},
		{"wrong type → nil", map[string]any{"outbound_header_removes": "notarray"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Account{Extra: tt.extra}
			got := a.GetOutboundHeaderRemoves()
			if len(got) != len(tt.expected) {
				t.Fatalf("len=%d want %d (%v)", len(got), len(tt.expected), got)
			}
			for i, v := range tt.expected {
				if got[i] != v {
					t.Errorf("[%d] = %q, want %q", i, got[i], v)
				}
			}
		})
	}
}

func TestApplyAccountOutboundHeaderOverrides(t *testing.T) {
	t.Run("override wins over existing mimic value", func(t *testing.T) {
		h := http.Header{}
		setHeaderRaw(h, "User-Agent", "claude-cli/2.1.185 (external, cli)")
		a := &Account{Extra: map[string]any{
			"outbound_header_overrides": map[string]any{"User-Agent": "custom-agent/1.0"},
		}}
		applyAccountOutboundHeaderOverrides(h, a)
		if got := getHeaderRaw(h, "User-Agent"); got != "custom-agent/1.0" {
			t.Errorf("User-Agent = %q, want custom-agent/1.0", got)
		}
	})

	t.Run("remove drops header in all forms", func(t *testing.T) {
		h := http.Header{}
		setHeaderRaw(h, "anthropic-beta", "some-beta")
		a := &Account{Extra: map[string]any{
			"outbound_header_removes": []any{"anthropic-beta"},
		}}
		applyAccountOutboundHeaderOverrides(h, a)
		if got := getHeaderRaw(h, "anthropic-beta"); got != "" {
			t.Errorf("anthropic-beta = %q, want empty", got)
		}
	})

	t.Run("cannot remove authorization (protected)", func(t *testing.T) {
		h := http.Header{}
		setHeaderRaw(h, "authorization", "Bearer secret")
		a := &Account{Extra: map[string]any{
			"outbound_header_removes":   []any{"authorization"},
			"outbound_header_overrides": map[string]any{"authorization": "Bearer hijack"},
		}}
		applyAccountOutboundHeaderOverrides(h, a)
		if got := getHeaderRaw(h, "authorization"); got != "Bearer secret" {
			t.Errorf("authorization = %q, want it untouched (Bearer secret)", got)
		}
	})

	t.Run("nil account no-op", func(t *testing.T) {
		h := http.Header{}
		setHeaderRaw(h, "X-Foo", "bar")
		applyAccountOutboundHeaderOverrides(h, nil)
		if got := getHeaderRaw(h, "X-Foo"); got != "bar" {
			t.Errorf("X-Foo = %q, want bar", got)
		}
	})
}
