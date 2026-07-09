package service

import (
	"net/http"
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
		platform, _, _ := canonicalEnvValues(c.stainlessOS)
		if platform != c.wantPlatform {
			t.Errorf("stainlessOS %q -> env platform %q, want %q", c.stainlessOS, platform, c.wantPlatform)
		}
	}
}
