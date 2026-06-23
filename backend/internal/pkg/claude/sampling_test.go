package claude

import "testing"

func TestModelRejectsSampling(t *testing.T) {
	cases := []struct {
		model string
		want  bool
	}{
		// Opus 4.5+ reject sampling — short names and dated IDs alike.
		{"claude-opus-4-5", true},
		{"claude-opus-4-5-20251101", true},
		{"claude-opus-4-6", true},
		{"claude-opus-4-7", true},
		{"claude-opus-4-7-20260620", true},
		{"claude-opus-4-8", true},
		{"claude-fable-5", true},
		{"CLAUDE-OPUS-4-7", true}, // case-insensitive
		{"  claude-opus-4-6  ", true}, // trimmed

		// Models that still accept sampling.
		{"claude-sonnet-4-5", false},
		{"claude-sonnet-4-5-20250929", false},
		{"claude-sonnet-4-6", false},
		{"claude-haiku-4-5", false},
		{"claude-opus-4-1", false}, // older opus still accepts
		{"claude-3-5-sonnet", false},
		{"", false},
		{"gpt-4o", false},
	}
	for _, c := range cases {
		if got := ModelRejectsSampling(c.model); got != c.want {
			t.Errorf("ModelRejectsSampling(%q) = %v, want %v", c.model, got, c.want)
		}
	}
}
