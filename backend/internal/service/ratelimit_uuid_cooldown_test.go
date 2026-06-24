//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// cooldownMockRepo 记录被 SetRateLimited 的账号 id，并按 uuid 返回兄弟记录。
type cooldownMockRepo struct {
	accountRepoStub
	byUUID      map[string][]Account
	rateLimited map[int64]time.Time
}

func (m *cooldownMockRepo) ListByUpstreamUUID(ctx context.Context, uuid string) ([]Account, error) {
	return m.byUUID[uuid], nil
}

func (m *cooldownMockRepo) SetRateLimited(ctx context.Context, id int64, resetAt time.Time) error {
	if m.rateLimited == nil {
		m.rateLimited = map[int64]time.Time{}
	}
	m.rateLimited[id] = resetAt
	return nil
}

func newRLSvc(repo AccountRepository, cooldownByUUID bool) *RateLimitService {
	return &RateLimitService{
		accountRepo: repo,
		cfg: &config.Config{Gateway: config.GatewayConfig{
			Scheduling: config.GatewaySchedulingConfig{CooldownByUUID: cooldownByUUID},
		}},
	}
}

func TestSetRateLimitedWithSiblings_LinksSameUUID(t *testing.T) {
	repo := &cooldownMockRepo{
		byUUID: map[string][]Account{
			"U": {{ID: 1}, {ID: 41}, {ID: 57}},
		},
	}
	svc := newRLSvc(repo, true)
	acc := &Account{ID: 1, Platform: PlatformAnthropic, Credentials: map[string]any{"account_uuid": "U"}}
	reset := time.Now().Add(time.Hour)

	err := svc.setRateLimitedWithSiblings(context.Background(), acc, reset)
	require.NoError(t, err)
	require.Contains(t, repo.rateLimited, int64(1), "primary must be cooled")
	require.Contains(t, repo.rateLimited, int64(41), "sibling 41 must be cooled")
	require.Contains(t, repo.rateLimited, int64(57), "sibling 57 must be cooled")
}

func TestSetRateLimitedWithSiblings_DisabledOnlyPrimary(t *testing.T) {
	repo := &cooldownMockRepo{
		byUUID: map[string][]Account{"U": {{ID: 1}, {ID: 41}}},
	}
	svc := newRLSvc(repo, false)
	acc := &Account{ID: 1, Platform: PlatformAnthropic, Credentials: map[string]any{"account_uuid": "U"}}

	err := svc.setRateLimitedWithSiblings(context.Background(), acc, time.Now().Add(time.Hour))
	require.NoError(t, err)
	require.Contains(t, repo.rateLimited, int64(1))
	require.NotContains(t, repo.rateLimited, int64(41), "siblings must NOT be cooled when disabled")
}

func TestSetRateLimitedWithSiblings_NoUUIDOnlyPrimary(t *testing.T) {
	repo := &cooldownMockRepo{byUUID: map[string][]Account{}}
	svc := newRLSvc(repo, true)
	acc := &Account{ID: 5, Platform: PlatformAnthropic} // 无 uuid

	err := svc.setRateLimitedWithSiblings(context.Background(), acc, time.Now().Add(time.Hour))
	require.NoError(t, err)
	require.Contains(t, repo.rateLimited, int64(5))
	require.Len(t, repo.rateLimited, 1)
}
