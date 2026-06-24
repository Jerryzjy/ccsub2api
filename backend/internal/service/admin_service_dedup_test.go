//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// dedupMockRepo 嵌入 accountRepoStub（满足完整接口），仅覆盖去重路径用到的方法。
type dedupMockRepo struct {
	accountRepoStub
	byUUID        map[string][]Account
	createCalls   int
	updCredCalls  int
	updExtraCalls int
	lastUpdID     int64
	getByIDResult *Account
}

func (m *dedupMockRepo) ListByUpstreamUUID(ctx context.Context, uuid string) ([]Account, error) {
	return m.byUUID[uuid], nil
}

func (m *dedupMockRepo) UpdateCredentials(ctx context.Context, id int64, credentials map[string]any) error {
	m.updCredCalls++
	m.lastUpdID = id
	return nil
}

func (m *dedupMockRepo) UpdateExtra(ctx context.Context, id int64, updates map[string]any) error {
	m.updExtraCalls++
	return nil
}

func (m *dedupMockRepo) Create(ctx context.Context, account *Account) error {
	m.createCalls++
	account.ID = 999
	return nil
}

func (m *dedupMockRepo) GetByID(ctx context.Context, id int64) (*Account, error) {
	if m.getByIDResult != nil {
		return m.getByIDResult, nil
	}
	return &Account{ID: id}, nil
}

func TestCreateAccount_DedupMergesExisting(t *testing.T) {
	repo := &dedupMockRepo{
		byUUID: map[string][]Account{
			"U": {{ID: 7, Name: "old", Platform: PlatformAnthropic, Type: AccountTypeOAuth,
				Credentials: map[string]any{"account_uuid": "U"}}},
		},
	}
	svc := &adminServiceImpl{accountRepo: repo, importDedupEnabled: true}

	out, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Name: "new", Platform: PlatformAnthropic, Type: AccountTypeOAuth,
		Credentials:          map[string]any{"account_uuid": "U", "access_token": "fresh"},
		SkipDefaultGroupBind: true, SkipMixedChannelCheck: true,
	})
	require.NoError(t, err)
	require.Equal(t, int64(7), out.ID, "should merge into existing id=7")
	require.Equal(t, 0, repo.createCalls, "must not create a new duplicate row")
	require.Equal(t, 1, repo.updCredCalls, "should update existing credentials")
	require.Equal(t, int64(7), repo.lastUpdID)
}

func TestCreateAccount_NoDupCreatesNew(t *testing.T) {
	repo := &dedupMockRepo{byUUID: map[string][]Account{}}
	svc := &adminServiceImpl{accountRepo: repo, importDedupEnabled: true}

	out, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Name: "fresh", Platform: PlatformAnthropic, Type: AccountTypeOAuth,
		Credentials:          map[string]any{"account_uuid": "NEW"},
		SkipDefaultGroupBind: true, SkipMixedChannelCheck: true,
	})
	require.NoError(t, err)
	require.Equal(t, int64(999), out.ID, "new uuid should create a new row")
	require.Equal(t, 1, repo.createCalls)
	require.Equal(t, 0, repo.updCredCalls)
}

func TestCreateAccount_DedupDisabledCreatesDuplicate(t *testing.T) {
	repo := &dedupMockRepo{
		byUUID: map[string][]Account{
			"U": {{ID: 7, Platform: PlatformAnthropic, Type: AccountTypeOAuth}},
		},
	}
	svc := &adminServiceImpl{accountRepo: repo, importDedupEnabled: false}

	_, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Name: "dup", Platform: PlatformAnthropic, Type: AccountTypeOAuth,
		Credentials:          map[string]any{"account_uuid": "U"},
		SkipDefaultGroupBind: true, SkipMixedChannelCheck: true,
	})
	require.NoError(t, err)
	require.Equal(t, 1, repo.createCalls, "dedup disabled should fall back to create")
}
