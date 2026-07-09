package service

import (
	"net/http"
	"strings"
	"testing"
)

func TestEnvOSProfileForSeed_Frozen(t *testing.T) {
	// Same seed must always yield the same profile (frozen per device_id).
	seed := "abc123def456"
	p1 := envOSProfileForSeed(seed)
	p2 := envOSProfileForSeed(seed)
	if p1 != p2 {
		t.Fatalf("profile not stable for same seed: %+v vs %+v", p1, p2)
	}
}

func TestEnvOSProfileForSeed_ValidValues(t *testing.T) {
	// Every produced profile must be one of the SDK-correct pairings.
	valid := map[string]string{
		"MacOS":   "arm64",
		"Linux":   "x64",
		"Windows": "x64",
	}
	for i := 0; i < 500; i++ {
		seed := "seed-" + string(rune('a'+i%26)) + string(rune('0'+i%10)) + string(rune(i))
		p := envOSProfileForSeed(seed)
		wantArch, ok := valid[p.XStainlessOS]
		if !ok {
			t.Fatalf("unexpected X-Stainless-OS %q (must be capitalized SDK name)", p.XStainlessOS)
		}
		if p.XStainlessArch != wantArch {
			t.Fatalf("OS %q got arch %q, want %q", p.XStainlessOS, p.XStainlessArch, wantArch)
		}
	}
}

func TestEnvOSProfileForSeed_EmptyFallback(t *testing.T) {
	p := envOSProfileForSeed("")
	if p.XStainlessOS != "Linux" || p.XStainlessArch != "x64" {
		t.Fatalf("empty seed fallback = %+v, want Linux/x64", p)
	}
}

func TestEnvOSProfileForSeed_DistributionSpread(t *testing.T) {
	// Across many seeds we must see more than one OS (i.e. the fleet is diversified,
	// not collapsed to a single OS).
	seen := map[string]int{}
	for i := 0; i < 1000; i++ {
		seed := "acct-device-" + string(rune(i)) + "-" + string(rune(i*7%128))
		seen[envOSProfileForSeed(seed).XStainlessOS]++
	}
	if len(seen) < 3 {
		t.Fatalf("expected all 3 OS classes to appear across 1000 seeds, got %v", seen)
	}
}

func TestApplyAccountEnvProfileHeaders(t *testing.T) {
	h := http.Header{}
	// simulate mimic having forced the global default first
	setHeaderRaw(h, "X-Stainless-OS", "Linux")
	setHeaderRaw(h, "X-Stainless-Arch", "arm64")

	applyAccountEnvProfileHeaders(h, envOSProfile{XStainlessOS: "MacOS", XStainlessArch: "arm64"})

	if got := getHeaderRaw(h, "X-Stainless-OS"); got != "MacOS" {
		t.Errorf("X-Stainless-OS = %q, want MacOS", got)
	}
	if got := getHeaderRaw(h, "X-Stainless-Arch"); got != "arm64" {
		t.Errorf("X-Stainless-Arch = %q, want arm64", got)
	}
}

// TestEnvOSProfile_LeavesRuntimeUntouched codifies the invariant that keeps the
// single Node TLS fingerprint valid across all OS profiles:
//
// TLS JA3 fingerprints the TLS *library* (Node/OpenSSL), not the OS. Real Claude
// Code sends the SAME JA3 on Windows/macOS/Linux (all node-24.x). So the OS
// profile must vary ONLY X-Stainless-OS/Arch and must NEVER touch the runtime
// identity (X-Stainless-Runtime / X-Stainless-Runtime-Version) that the TLS
// fingerprint is keyed to. If a future change makes applyAccountEnvProfileHeaders
// mutate the runtime headers, this test fails on purpose — do not "fix" it by
// adding per-OS TLS; fix the mutation.
func TestEnvOSProfile_LeavesRuntimeUntouched(t *testing.T) {
	for _, seed := range []string{"seed-mac", "seed-win", "seed-linux", "deadbeef", ""} {
		h := http.Header{}
		setHeaderRaw(h, "X-Stainless-Runtime", "node")
		setHeaderRaw(h, "X-Stainless-Runtime-Version", "v24.3.0")

		applyAccountEnvProfileHeaders(h, envOSProfileForSeed(seed))

		if got := getHeaderRaw(h, "X-Stainless-Runtime"); got != "node" {
			t.Errorf("seed %q: X-Stainless-Runtime mutated to %q (must stay node; TLS is keyed to runtime, not OS)", seed, got)
		}
		if got := getHeaderRaw(h, "X-Stainless-Runtime-Version"); got != "v24.3.0" {
			t.Errorf("seed %q: X-Stainless-Runtime-Version mutated to %q (must stay uniform to match the single node TLS JA3)", seed, got)
		}
	}
}

// TestEnvProfileHeadBodyConsistency verifies the header OS and the <env> block
// platform stay consistent (capitalized header vs lowercase env platform).
func TestEnvProfileHeadBodyConsistency(t *testing.T) {
	cases := []struct {
		stainlessOS  string
		wantPlatform string
	}{
		{"MacOS", "darwin"},
		{"Windows", "win32"},
		{"Linux", "linux"},
	}
	for _, c := range cases {
		_, platform, _, _ := canonicalEnvValues(c.stainlessOS)
		if platform != c.wantPlatform {
			t.Errorf("stainlessOS %q -> env platform %q, want %q", c.stainlessOS, platform, c.wantPlatform)
		}
	}
}

// TestCanonicalWorkingDirConsistency: Working directory path must be OS-correct
// and never contain real user names.
func TestCanonicalWorkingDirConsistency(t *testing.T) {
	cases := []struct {
		stainlessOS    string
		wantWorkingDir string
	}{
		{"MacOS", "/Users/dev/project"},
		{"Windows", `C:\Users\dev\project`},
		{"Linux", "/home/dev/project"},
	}
	for _, c := range cases {
		wd, _, _, _ := canonicalEnvValues(c.stainlessOS)
		if wd != c.wantWorkingDir {
			t.Errorf("stainlessOS %q -> working dir %q, want %q", c.stainlessOS, wd, c.wantWorkingDir)
		}
	}
}

// TestSanitizeStripsLeakedTimezone: 下游客户端经常在 <env> 里塞 "Timezone: Asia/Shanghai"
// 之类的行（真 CLI 不发这个字段）。这种行必须剥离，且不能伪造任何 timezone 替代。
func TestSanitizeStripsLeakedTimezone(t *testing.T) {
	in := "<env>\nWorking directory: /Users/dev/project\nPlatform: darwin\nOS Version: Darwin 24.4.0\nShell: zsh\nTimezone: Asia/Shanghai\n</env>"
	got := sanitizeMachineEnvText(in, "/Users/dev/project", "darwin", "Darwin 24.4.0", "zsh")
	if strings.Contains(got, "Timezone:") || strings.Contains(got, "Asia/Shanghai") {
		t.Errorf("timezone line leaked to upstream: %q", got)
	}
	// CLI 真字段仍需被保留。
	for _, want := range []string{"Platform: darwin", "OS Version: Darwin 24.4.0", "Shell: zsh"} {
		if !strings.Contains(got, want) {
			t.Errorf("cli field dropped during strip: %q (want %s)", got, want)
		}
	}
}

// TestSanitizeStripsTimezoneInSystemReminder: <system-reminder> 块里的 Timezone 也要剥，
// 不光 <env>。
func TestSanitizeStripsTimezoneInSystemReminder(t *testing.T) {
	in := "<system-reminder>Platform: darwin\nTimezone: America/New_York\nShell: zsh</system-reminder>"
	got := sanitizeMachineEnvText(in, "/Users/dev/project", "darwin", "Darwin 24.4.0", "zsh")
	if strings.Contains(got, "Timezone:") || strings.Contains(got, "America/New_York") {
		t.Errorf("timezone leaked from system-reminder: %q", got)
	}
}

// TestSanitizeTimezoneStripIsNotAdded: 确保我们真的没伪造 timezone 上去。
// 没有 Timezone 行的情况下，输出也不能凭空多一行出来。
func TestSanitizeTimezoneStripIsNotAdded(t *testing.T) {
	in := "<env>\nPlatform: linux\nOS Version: Linux 6.8.0-45-generic\nShell: bash\n</env>"
	got := sanitizeMachineEnvText(in, "/home/dev/project", "linux", "Linux 6.8.0-45-generic", "bash")
	if strings.Contains(got, "Timezone:") {
		t.Errorf("sanitizer must NOT inject a timezone line: %q", got)
	}
}
