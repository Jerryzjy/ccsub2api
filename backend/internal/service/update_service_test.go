//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type updateServiceCacheStub struct {
	data string
}

func (s *updateServiceCacheStub) GetUpdateInfo(context.Context) (string, error) {
	if s.data == "" {
		return "", errors.New("cache miss")
	}
	return s.data, nil
}

func (s *updateServiceCacheStub) SetUpdateInfo(_ context.Context, data string, _ time.Duration) error {
	s.data = data
	return nil
}

type updateServiceGitHubClientStub struct {
	release *GitHubRelease
}

func (s *updateServiceGitHubClientStub) FetchLatestRelease(context.Context, string) (*GitHubRelease, error) {
	return s.release, nil
}

func (s *updateServiceGitHubClientStub) DownloadFile(context.Context, string, string, int64) error {
	panic("DownloadFile should not be called when no update is available")
}

func (s *updateServiceGitHubClientStub) FetchChecksumFile(context.Context, string) ([]byte, error) {
	panic("FetchChecksumFile should not be called when no update is available")
}

// downloadFailGitHubClientStub 让 DownloadFile 返回错误，用于验证更新失败的真实原因被透传。
type downloadFailGitHubClientStub struct {
	release *GitHubRelease
}

func (s *downloadFailGitHubClientStub) FetchLatestRelease(context.Context, string) (*GitHubRelease, error) {
	return s.release, nil
}

func (s *downloadFailGitHubClientStub) DownloadFile(context.Context, string, string, int64) error {
	return errors.New("connection refused")
}

func (s *downloadFailGitHubClientStub) FetchChecksumFile(context.Context, string) ([]byte, error) {
	return nil, errors.New("should not reach checksum")
}

func TestUpdateServicePerformUpdateDownloadFailureSurfacesCause(t *testing.T) {
	archive := fmt.Sprintf("sub2api_9.9.9_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	svc := NewUpdateService(
		&updateServiceCacheStub{},
		&downloadFailGitHubClientStub{
			release: &GitHubRelease{
				TagName: "v9.9.9",
				Name:    "v9.9.9",
				Assets: []GitHubAsset{
					{Name: archive, BrowserDownloadURL: "https://github.com/owner/repo/releases/download/v9.9.9/" + archive},
					{Name: "checksums.txt", BrowserDownloadURL: "https://github.com/owner/repo/releases/download/v9.9.9/checksums.txt"},
				},
			},
		},
		"1.0.0",
		"release",
	)

	err := svc.PerformUpdate(context.Background())
	require.Error(t, err)
	// 失败原因必须可见（前端不应只看到通用 "internal error"）。
	require.Contains(t, err.Error(), "download")
	require.Contains(t, err.Error(), "connection refused")
	// 必须是结构化错误，使其 message 能透传到 HTTP 响应。
	var appErr *infraerrors.ApplicationError
	require.True(t, errors.As(err, &appErr), "expected ApplicationError, got %T", err)
	require.NotEqual(t, infraerrors.UnknownMessage, appErr.Message)
}

func TestUpdateServicePerformUpdateNoUpdateReturnsSentinel(t *testing.T) {
	svc := NewUpdateService(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{
			release: &GitHubRelease{
				TagName: "v0.1.132",
				Name:    "v0.1.132",
			},
		},
		"0.1.132",
		"release",
	)

	err := svc.PerformUpdate(context.Background())

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNoUpdateAvailable))
	require.ErrorIs(t, err, ErrNoUpdateAvailable)
}
