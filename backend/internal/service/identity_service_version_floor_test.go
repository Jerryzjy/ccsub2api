package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"

	"github.com/stretchr/testify/require"
)

// cachedFingerprintStub 返回一个预置的缓存指纹，并记录回写内容。
type cachedFingerprintStub struct {
	cached  *Fingerprint
	written *Fingerprint
}

func (s *cachedFingerprintStub) GetFingerprint(_ context.Context, _ int64) (*Fingerprint, error) {
	return s.cached, nil
}
func (s *cachedFingerprintStub) SetFingerprint(_ context.Context, _ int64, fp *Fingerprint) error {
	s.written = fp
	return nil
}
func (s *cachedFingerprintStub) GetMaskedSessionID(_ context.Context, _ int64) (string, error) {
	return "", nil
}
func (s *cachedFingerprintStub) SetMaskedSessionID(_ context.Context, _ int64, _ string) error {
	return nil
}

func newCachedFingerprint(ua string) *Fingerprint {
	return &Fingerprint{
		ClientID:                "device-id-stays-put",
		UserAgent:               ua,
		StainlessLang:           "js",
		StainlessPackageVersion: "0.106.0",
		StainlessOS:             "Darwin",
		StainlessArch:           "arm64",
		StainlessRuntime:        "node",
		StainlessRuntimeVersion: "v24.3.0",
		UpdatedAt:               time.Now().Unix(),
	}
}

// 旧账号的缓存指纹停留在过期 CLI 版本上时，会被 Anthropic 以
// claude_code_version_too_old 拒绝新模型；必须抬到 CLICurrentVersion。
func TestGetOrCreateFingerprint_BumpsStaleUAToCurrentVersion(t *testing.T) {
	stub := &cachedFingerprintStub{cached: newCachedFingerprint("claude-cli/2.1.185 (external, cli)")}
	svc := NewIdentityService(stub)

	fp, err := svc.GetOrCreateFingerprint(context.Background(), 1, http.Header{})
	require.NoError(t, err)

	require.Equal(t, claude.DefaultHeaders["User-Agent"], fp.UserAgent)
	require.Equal(t, claude.CLICurrentVersion, ExtractCLIVersion(fp.UserAgent))
	require.NotNil(t, stub.written, "bumped fingerprint must be persisted")
	require.Equal(t, claude.DefaultHeaders["User-Agent"], stub.written.UserAgent)

	// 设备特征不得随版本地板漂移
	require.Equal(t, "device-id-stays-put", fp.ClientID)
	require.Equal(t, "Darwin", fp.StainlessOS)
}

// 客户端本身就比内置版本新时，不得被地板回退。
func TestGetOrCreateFingerprint_DoesNotDowngradeNewerUA(t *testing.T) {
	newerUA := "claude-cli/9.9.9 (external, cli)"
	stub := &cachedFingerprintStub{cached: newCachedFingerprint(newerUA)}
	svc := NewIdentityService(stub)

	fp, err := svc.GetOrCreateFingerprint(context.Background(), 1, http.Header{})
	require.NoError(t, err)
	require.Equal(t, newerUA, fp.UserAgent)
}
