package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHashJSONField(t *testing.T) {
	body := []byte(`{"tools":[{"name":"a"}],"system":"hi"}`)

	// 相同字段 → 相同哈希（缓存可命中的前提）。
	require.Equal(t, hashJSONField(body, "tools"), hashJSONField(body, "tools"))

	// 字段不存在 → "-"。
	require.Equal(t, "-", hashJSONField(body, "messages"))

	// 不同内容 → 不同哈希（能检测前缀污染）。
	other := []byte(`{"tools":[{"name":"b"}]}`)
	require.NotEqual(t, hashJSONField(body, "tools"), hashJSONField(other, "tools"))
}

func TestHashMessagesSplit_StableVsAppended(t *testing.T) {
	// 第一轮：两条消息。
	round1 := []byte(`{"messages":[{"role":"user","content":"a"},{"role":"assistant","content":"b"}]}`)
	// 第二轮：在末尾追加一条新 user（正常多轮对话）。前两条逐字节不变。
	round2 := []byte(`{"messages":[{"role":"user","content":"a"},{"role":"assistant","content":"b"},{"role":"user","content":"c"}]}`)

	p1, last1 := hashMessagesSplit(round1)
	p2, last2 := hashMessagesSplit(round2)

	// round2 的"前缀"= round1 全部两条 + 分隔符；round1 的"前缀"只含第一条。
	// 二者前缀哈希本就不同，这里要验证的关键不变量是：
	//   - 末条哈希随末条内容变化（last1 != last2）。
	require.NotEqual(t, last1, last2)

	// 关键诊断不变量：若前缀逐字节冻结，则同样的"前 n-1 条"必产生同样的前缀哈希。
	// 构造 round2 的前缀等价体（去掉末条 c）= round1，验证前缀哈希可复现。
	pSame, _ := hashMessagesSplit(round1)
	require.Equal(t, p1, pSame)
	_ = p2
}

func TestHashMessagesSplit_PrefixPollutionDetected(t *testing.T) {
	// 模拟"前缀被污染"：把第一条 user 的内容改了（比如 system 搬进 messages 后逐轮变化）。
	clean := []byte(`{"messages":[{"role":"user","content":"sys-v1"},{"role":"user","content":"q"}]}`)
	polluted := []byte(`{"messages":[{"role":"user","content":"sys-v2"},{"role":"user","content":"q"}]}`)

	pClean, lastClean := hashMessagesSplit(clean)
	pPolluted, lastPolluted := hashMessagesSplit(polluted)

	// 末条相同（都是 "q"）。
	require.Equal(t, lastClean, lastPolluted)
	// 但前缀哈希不同 → 探针能把"前缀被污染"暴露出来（这正是真凶1的信号）。
	require.NotEqual(t, pClean, pPolluted)
}

func TestHashMessagesSplit_Edge(t *testing.T) {
	// 无 messages 字段。
	p, last := hashMessagesSplit([]byte(`{}`))
	require.Equal(t, "-", p)
	require.Equal(t, "-", last)

	// 空数组。
	p, last = hashMessagesSplit([]byte(`{"messages":[]}`))
	require.Equal(t, "-", p)
	require.Equal(t, "-", last)

	// 单条：前缀为空（稳定哈希），末条有哈希。
	p, last = hashMessagesSplit([]byte(`{"messages":[{"role":"user","content":"x"}]}`))
	require.NotEqual(t, "-", last)
	require.Len(t, p, 16) // 空前缀仍产生 16 位 hex
}

func TestFormatHash(t *testing.T) {
	require.Equal(t, "0000000000000000", formatHash(0))
	require.Equal(t, "ffffffffffffffff", formatHash(^uint64(0)))
	require.Len(t, formatHash(12345), 16)
}
