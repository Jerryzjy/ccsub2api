//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFilterByMinPriority(t *testing.T) {
	t.Run("empty slice", func(t *testing.T) {
		result := filterByMinPriority(nil)
		require.Empty(t, result)
	})

	t.Run("single account", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, Priority: 5}, loadInfo: &AccountLoadInfo{}},
		}
		result := filterByMinPriority(accounts)
		require.Len(t, result, 1)
		require.Equal(t, int64(1), result[0].account.ID)
	})

	t.Run("multiple accounts same priority", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, Priority: 3}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 2, Priority: 3}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 3, Priority: 3}, loadInfo: &AccountLoadInfo{}},
		}
		result := filterByMinPriority(accounts)
		require.Len(t, result, 3)
	})

	t.Run("filters to min priority only", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, Priority: 5}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 2, Priority: 1}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 3, Priority: 3}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 4, Priority: 1}, loadInfo: &AccountLoadInfo{}},
		}
		result := filterByMinPriority(accounts)
		require.Len(t, result, 2)
		require.Equal(t, int64(2), result[0].account.ID)
		require.Equal(t, int64(4), result[1].account.ID)
	})
}

func TestFilterByMinLoadRate(t *testing.T) {
	t.Run("empty slice", func(t *testing.T) {
		result := filterByMinLoadRate(nil)
		require.Empty(t, result)
	})

	t.Run("single account", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{LoadRate: 50}},
		}
		result := filterByMinLoadRate(accounts)
		require.Len(t, result, 1)
		require.Equal(t, int64(1), result[0].account.ID)
	})

	t.Run("multiple accounts same load rate", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{LoadRate: 20}},
			{account: &Account{ID: 2}, loadInfo: &AccountLoadInfo{LoadRate: 20}},
			{account: &Account{ID: 3}, loadInfo: &AccountLoadInfo{LoadRate: 20}},
		}
		result := filterByMinLoadRate(accounts)
		require.Len(t, result, 3)
	})

	t.Run("filters to min load rate only", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{LoadRate: 80}},
			{account: &Account{ID: 2}, loadInfo: &AccountLoadInfo{LoadRate: 10}},
			{account: &Account{ID: 3}, loadInfo: &AccountLoadInfo{LoadRate: 50}},
			{account: &Account{ID: 4}, loadInfo: &AccountLoadInfo{LoadRate: 10}},
		}
		result := filterByMinLoadRate(accounts)
		require.Len(t, result, 2)
		require.Equal(t, int64(2), result[0].account.ID)
		require.Equal(t, int64(4), result[1].account.ID)
	})

	t.Run("zero load rate", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{LoadRate: 0}},
			{account: &Account{ID: 2}, loadInfo: &AccountLoadInfo{LoadRate: 50}},
			{account: &Account{ID: 3}, loadInfo: &AccountLoadInfo{LoadRate: 0}},
		}
		result := filterByMinLoadRate(accounts)
		require.Len(t, result, 2)
		require.Equal(t, int64(1), result[0].account.ID)
		require.Equal(t, int64(3), result[1].account.ID)
	})
}

func TestSelectByLRU(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-1 * time.Hour)
	muchEarlier := now.Add(-2 * time.Hour)

	t.Run("empty slice", func(t *testing.T) {
		result := selectByLRU(nil, false)
		require.Nil(t, result)
	})

	t.Run("single account", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, LastUsedAt: &now}, loadInfo: &AccountLoadInfo{}},
		}
		result := selectByLRU(accounts, false)
		require.NotNil(t, result)
		require.Equal(t, int64(1), result.account.ID)
	})

	t.Run("selects least recently used", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, LastUsedAt: &now}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 2, LastUsedAt: &muchEarlier}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 3, LastUsedAt: &earlier}, loadInfo: &AccountLoadInfo{}},
		}
		result := selectByLRU(accounts, false)
		require.NotNil(t, result)
		require.Equal(t, int64(2), result.account.ID)
	})

	t.Run("nil LastUsedAt preferred over non-nil", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, LastUsedAt: &now}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 2, LastUsedAt: nil}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 3, LastUsedAt: &earlier}, loadInfo: &AccountLoadInfo{}},
		}
		result := selectByLRU(accounts, false)
		require.NotNil(t, result)
		require.Equal(t, int64(2), result.account.ID)
	})

	t.Run("multiple nil LastUsedAt random selection", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, LastUsedAt: nil, Type: "session"}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 2, LastUsedAt: nil, Type: "session"}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 3, LastUsedAt: nil, Type: "session"}, loadInfo: &AccountLoadInfo{}},
		}
		// 多次调用应该随机选择，验证结果都在候选范围内
		validIDs := map[int64]bool{1: true, 2: true, 3: true}
		for i := 0; i < 10; i++ {
			result := selectByLRU(accounts, false)
			require.NotNil(t, result)
			require.True(t, validIDs[result.account.ID], "selected ID should be one of the candidates")
		}
	})

	t.Run("multiple same LastUsedAt random selection", func(t *testing.T) {
		sameTime := now
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, LastUsedAt: &sameTime}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 2, LastUsedAt: &sameTime}, loadInfo: &AccountLoadInfo{}},
		}
		// 多次调用应该随机选择
		validIDs := map[int64]bool{1: true, 2: true}
		for i := 0; i < 10; i++ {
			result := selectByLRU(accounts, false)
			require.NotNil(t, result)
			require.True(t, validIDs[result.account.ID], "selected ID should be one of the candidates")
		}
	})

	t.Run("preferOAuth selects from OAuth accounts when multiple nil", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, LastUsedAt: nil, Type: "session"}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 2, LastUsedAt: nil, Type: AccountTypeOAuth}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 3, LastUsedAt: nil, Type: AccountTypeOAuth}, loadInfo: &AccountLoadInfo{}},
		}
		// preferOAuth 时，应该从 OAuth 类型中选择
		oauthIDs := map[int64]bool{2: true, 3: true}
		for i := 0; i < 10; i++ {
			result := selectByLRU(accounts, true)
			require.NotNil(t, result)
			require.True(t, oauthIDs[result.account.ID], "should select from OAuth accounts")
		}
	})

	t.Run("preferOAuth falls back to all when no OAuth", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, LastUsedAt: nil, Type: "session"}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 2, LastUsedAt: nil, Type: "session"}, loadInfo: &AccountLoadInfo{}},
		}
		// 没有 OAuth 时，从所有候选中选择
		validIDs := map[int64]bool{1: true, 2: true}
		for i := 0; i < 10; i++ {
			result := selectByLRU(accounts, true)
			require.NotNil(t, result)
			require.True(t, validIDs[result.account.ID])
		}
	})

	t.Run("preferOAuth only affects same LastUsedAt accounts", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, LastUsedAt: &earlier, Type: "session"}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 2, LastUsedAt: &now, Type: AccountTypeOAuth}, loadInfo: &AccountLoadInfo{}},
		}
		result := selectByLRU(accounts, true)
		require.NotNil(t, result)
		// 有不同 LastUsedAt 时，按时间选择最早的，不受 preferOAuth 影响
		require.Equal(t, int64(1), result.account.ID)
	})
}

func TestLayeredFilterIntegration(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-1 * time.Hour)
	muchEarlier := now.Add(-2 * time.Hour)

	t.Run("full layered selection", func(t *testing.T) {
		// 模拟真实场景：多个账号，不同优先级、负载率、最后使用时间
		accounts := []accountWithLoad{
			// 优先级 1，负载 50%
			{account: &Account{ID: 1, Priority: 1, LastUsedAt: &now}, loadInfo: &AccountLoadInfo{LoadRate: 50}},
			// 优先级 1，负载 20%（最低）
			{account: &Account{ID: 2, Priority: 1, LastUsedAt: &earlier}, loadInfo: &AccountLoadInfo{LoadRate: 20}},
			// 优先级 1，负载 20%（最低），更早使用
			{account: &Account{ID: 3, Priority: 1, LastUsedAt: &muchEarlier}, loadInfo: &AccountLoadInfo{LoadRate: 20}},
			// 优先级 2（较低优先）
			{account: &Account{ID: 4, Priority: 2, LastUsedAt: &muchEarlier}, loadInfo: &AccountLoadInfo{LoadRate: 0}},
		}

		// 1. 取优先级最小的集合 → ID: 1, 2, 3
		step1 := filterByMinPriority(accounts)
		require.Len(t, step1, 3)

		// 2. 取负载率最低的集合 → ID: 2, 3
		step2 := filterByMinLoadRate(step1)
		require.Len(t, step2, 2)

		// 3. LRU 选择 → ID: 3（muchEarlier 最早）
		selected := selectByLRU(step2, false)
		require.NotNil(t, selected)
		require.Equal(t, int64(3), selected.account.ID)
	})

	t.Run("all same priority and load rate", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, Priority: 1, LastUsedAt: &now}, loadInfo: &AccountLoadInfo{LoadRate: 50}},
			{account: &Account{ID: 2, Priority: 1, LastUsedAt: &earlier}, loadInfo: &AccountLoadInfo{LoadRate: 50}},
			{account: &Account{ID: 3, Priority: 1, LastUsedAt: &muchEarlier}, loadInfo: &AccountLoadInfo{LoadRate: 50}},
		}

		step1 := filterByMinPriority(accounts)
		require.Len(t, step1, 3)

		step2 := filterByMinLoadRate(step1)
		require.Len(t, step2, 3)

		// LRU 选择最早的
		selected := selectByLRU(step2, false)
		require.NotNil(t, selected)
		require.Equal(t, int64(3), selected.account.ID)
	})
}

func TestSelectRoundRobin(t *testing.T) {
	t.Run("empty slice", func(t *testing.T) {
		accountRoundRobinCounter.Store(0)
		result := selectRoundRobin(nil, false)
		require.Nil(t, result)
	})

	t.Run("single account", func(t *testing.T) {
		accountRoundRobinCounter.Store(0)
		accounts := []accountWithLoad{
			{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{}},
		}
		result := selectRoundRobin(accounts, false)
		require.NotNil(t, result)
		require.Equal(t, int64(1), result.account.ID)
	})

	// 均匀分布：N 个同优先级同负载、LastUsedAt 各异的账号，M 次调用应接近均匀。
	// 这是修复"串行流量下榨干单账号"的核心断言——LastUsedAt 各异也不应塌缩。
	t.Run("uniform distribution ignores LastUsedAt", func(t *testing.T) {
		accountRoundRobinCounter.Store(0)
		now := time.Now()
		accounts := make([]accountWithLoad, 0, 5)
		for i := int64(1); i <= 5; i++ {
			used := now.Add(-time.Duration(i) * time.Minute) // 各异的 LastUsedAt
			accounts = append(accounts, accountWithLoad{
				account:  &Account{ID: i, Priority: 50, LastUsedAt: &used},
				loadInfo: &AccountLoadInfo{LoadRate: 0},
			})
		}

		counts := make(map[int64]int)
		for i := 0; i < 50; i++ {
			selected := selectRoundRobin(accounts, false)
			require.NotNil(t, selected)
			counts[selected.account.ID]++
		}

		require.Len(t, counts, 5, "所有账号都应被选中")
		for id, c := range counts {
			require.GreaterOrEqual(t, c, 9, "账号 %d 选中次数过少: %d", id, c)
			require.LessOrEqual(t, c, 11, "账号 %d 选中次数过多: %d", id, c)
		}
	})

	// 跨请求记忆：连续两次调用同一集合应返回不同账号（区别于随机可能重复）。
	t.Run("counter advances across calls", func(t *testing.T) {
		accountRoundRobinCounter.Store(0)
		accounts := []accountWithLoad{
			{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 2}, loadInfo: &AccountLoadInfo{}},
		}
		first := selectRoundRobin(accounts, false)
		second := selectRoundRobin(accounts, false)
		require.NotNil(t, first)
		require.NotNil(t, second)
		require.NotEqual(t, first.account.ID, second.account.ID)
	})

	// preferOAuth：仅在 OAuth 子集内轮询，且其间均匀。
	t.Run("preferOAuth rotates only within OAuth accounts", func(t *testing.T) {
		accountRoundRobinCounter.Store(0)
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, Type: "session"}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 2, Type: AccountTypeOAuth}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 3, Type: "session"}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 4, Type: AccountTypeOAuth}, loadInfo: &AccountLoadInfo{}},
		}

		counts := make(map[int64]int)
		for i := 0; i < 20; i++ {
			selected := selectRoundRobin(accounts, true)
			require.NotNil(t, selected)
			require.Equal(t, AccountTypeOAuth, selected.account.Type, "preferOAuth 时只应返回 OAuth 账号")
			counts[selected.account.ID]++
		}
		require.Equal(t, 10, counts[2])
		require.Equal(t, 10, counts[4])
	})

	// preferOAuth 但无 OAuth 账号时，回退到全集轮询。
	t.Run("preferOAuth falls back to full pool when no OAuth", func(t *testing.T) {
		accountRoundRobinCounter.Store(0)
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, Type: "session"}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 2, Type: "session"}, loadInfo: &AccountLoadInfo{}},
		}
		counts := make(map[int64]int)
		for i := 0; i < 10; i++ {
			selected := selectRoundRobin(accounts, true)
			require.NotNil(t, selected)
			counts[selected.account.ID]++
		}
		require.Equal(t, 5, counts[1])
		require.Equal(t, 5, counts[2])
	})
}

func TestSelectTieBreakRouting(t *testing.T) {
	accs := []accountWithLoad{
		{account: &Account{ID: 1, Type: AccountTypeOAuth}, loadInfo: &AccountLoadInfo{}},
		{account: &Account{ID: 2, Type: AccountTypeOAuth}, loadInfo: &AccountLoadInfo{}},
	}

	// lru 模式应返回非 nil（确定性，不依赖全局计数器）
	require.NotNil(t, selectTieBreak(accs, false, "lru"))

	// 默认（空 mode）应等同 lru，不轮询
	require.NotNil(t, selectTieBreak(accs, false, ""))

	// round_robin 模式连续两次应轮换到不同账号
	a := selectTieBreak(accs, false, "round_robin")
	b := selectTieBreak(accs, false, "round_robin")
	require.NotNil(t, a)
	require.NotNil(t, b)
	require.NotEqual(t, a.account.ID, b.account.ID, "round_robin should rotate")
}
