package service

import (
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

func TestCanonicalEnvValues(t *testing.T) {
	tests := []struct {
		os       string
		platform string
	}{
		{"Linux", "linux"},
		{"", "linux"},
		{"unknown", "linux"},
		{"MacOS", "darwin"},
		{"darwin", "darwin"},
		{"Windows", "win32"},
		{"win32", "win32"},
	}
	for _, tt := range tests {
		if p, _, _ := canonicalEnvValues(tt.os); p != tt.platform {
			t.Errorf("canonicalEnvValues(%q) platform = %q, want %q", tt.os, p, tt.platform)
		}
	}
}

func TestSanitizeMachineEnvText_OnlyInsideBlocks(t *testing.T) {
	// Field outside a delimited block must NOT be touched (user free text).
	free := "The user said Platform: darwin in their message."
	if got := sanitizeMachineEnvText(free, "linux", "Linux 6.8.0-45-generic", "bash"); got != free {
		t.Errorf("free text was modified: %q", got)
	}

	// Inside <env> block: rewritten to canonical Linux.
	in := "<env>\nPlatform: darwin\nOS Version: Darwin 24.4.0\nShell: zsh\n</env>"
	got := sanitizeMachineEnvText(in, "linux", "Linux 6.8.0-45-generic", "bash")
	for _, want := range []string{"Platform: linux", "OS Version: Linux 6.8.0-45-generic", "Shell: bash"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
	for _, bad := range []string{"darwin", "Darwin 24.4.0", "zsh"} {
		if strings.Contains(got, bad) {
			t.Errorf("leaked %q in %q", bad, got)
		}
	}
}

func TestSanitizeMachineEnvText_SystemReminder(t *testing.T) {
	in := "prefix <system-reminder>Platform: win32\nShell: powershell</system-reminder> suffix"
	got := sanitizeMachineEnvText(in, "linux", "Linux 6.8.0-45-generic", "bash")
	if !strings.Contains(got, "Platform: linux") || !strings.Contains(got, "Shell: bash") {
		t.Errorf("system-reminder not normalized: %q", got)
	}
	if !strings.HasPrefix(got, "prefix ") || !strings.HasSuffix(got, " suffix") {
		t.Errorf("surrounding text altered: %q", got)
	}
}

func TestSanitizeSystemMachineEnv_ArrayBody(t *testing.T) {
	body := []byte(`{"system":[{"type":"text","text":"You are Claude Code"},{"type":"text","text":"<env>\nPlatform: darwin\nOS Version: Darwin 24.4.0\n</env>"}],"messages":[]}`)
	out := sanitizeSystemMachineEnv(body, "Linux")
	block1 := gjson.GetBytes(out, "system.1.text").String()
	if !strings.Contains(block1, "Platform: linux") {
		t.Errorf("array env not normalized: %q", block1)
	}
	if strings.Contains(block1, "darwin") {
		t.Errorf("darwin leaked: %q", block1)
	}
	// First block (no env) untouched.
	if gjson.GetBytes(out, "system.0.text").String() != "You are Claude Code" {
		t.Errorf("non-env block was altered")
	}
}

func TestSanitizeSystemMachineEnv_StringBody(t *testing.T) {
	body := []byte(`{"system":"banner\n<env>\nPlatform: win32\n</env>","messages":[]}`)
	out := sanitizeSystemMachineEnv(body, "Linux")
	sys := gjson.GetBytes(out, "system").String()
	if !strings.Contains(sys, "Platform: linux") || strings.Contains(sys, "win32") {
		t.Errorf("string system not normalized: %q", sys)
	}
}

func TestSanitizeSystemMachineEnv_NoSystem(t *testing.T) {
	body := []byte(`{"messages":[]}`)
	out := sanitizeSystemMachineEnv(body, "Linux")
	if string(out) != string(body) {
		t.Errorf("body without system was modified")
	}
}
