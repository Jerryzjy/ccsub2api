package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRecordStickySwitch(t *testing.T) {
	snapshot := func() (int64, int64, int64) {
		s, c, n, _ := GatewayStickySwitchStats()
		return s, c, n
	}

	baseSame, baseChanged, baseNoBinding := snapshot()

	// 无绑定（首轮）：计入 noBinding。
	recordStickySwitch(0, 42, "sess-a", "")
	s, c, n := snapshot()
	require.Equal(t, baseSame, s)
	require.Equal(t, baseChanged, c)
	require.Equal(t, baseNoBinding+1, n)

	// 同账号：计入 same。
	recordStickySwitch(42, 42, "sess-a", "cache")
	s, c, n = snapshot()
	require.Equal(t, baseSame+1, s)
	require.Equal(t, baseChanged, c)
	require.Equal(t, baseNoBinding+1, n)

	// 切号：计入 changed（缓存 miss 来源）。
	recordStickySwitch(42, 99, "sess-a", "cache")
	s, c, n = snapshot()
	require.Equal(t, baseSame+1, s)
	require.Equal(t, baseChanged+1, c)
	require.Equal(t, baseNoBinding+1, n)

	// 本轮未选出账号（<=0）：任何计数都不增加。
	recordStickySwitch(42, 0, "sess-a", "cache")
	s, c, n = snapshot()
	require.Equal(t, baseSame+1, s)
	require.Equal(t, baseChanged+1, c)
	require.Equal(t, baseNoBinding+1, n)
}

func TestGatewayStickySwitchStats_SwitchRate(t *testing.T) {
	// switchRate = changed / (same + changed)。由于计数器是包级全局且其他测试也会写，
	// 这里只验证比率公式在已知增量下自洽，而非断言绝对值。
	s0, c0, _, _ := GatewayStickySwitchStats()

	recordStickySwitch(1, 1, "r", "cache") // same +1
	recordStickySwitch(1, 2, "r", "cache") // changed +1
	recordStickySwitch(1, 3, "r", "cache") // changed +1

	s1, c1, _, rate := GatewayStickySwitchStats()
	require.Equal(t, s0+1, s1)
	require.Equal(t, c0+2, c1)

	expected := float64(c1) / float64(s1+c1)
	require.InDelta(t, expected, rate, 1e-9)
}
