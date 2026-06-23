//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFindDanglingToolRefs_None(t *testing.T) {
	// tool_use 引用的 WebSearch 已在 tools 声明 → 无悬空。
	body := []byte(`{
		"tools":[{"name":"WebSearch","type":"custom"}],
		"messages":[{"role":"assistant","content":[
			{"type":"tool_use","id":"t1","name":"WebSearch","input":{}}]}]
	}`)
	require.Empty(t, findDanglingToolRefs(body))
}

func TestFindDanglingToolRefs_ToolUseDangling(t *testing.T) {
	// 历史 tool_use 引用 WebSearch，但本轮 tools 没声明 → 悬空。
	body := []byte(`{
		"tools":[{"name":"Read","type":"custom"}],
		"messages":[{"role":"assistant","content":[
			{"type":"tool_use","id":"t1","name":"WebSearch","input":{}}]}]
	}`)
	require.Equal(t, []string{"WebSearch"}, findDanglingToolRefs(body))
}

func TestFindDanglingToolRefs_ToolChoiceDangling(t *testing.T) {
	body := []byte(`{
		"tools":[{"name":"Read","type":"custom"}],
		"tool_choice":{"type":"tool","name":"WebSearch"},
		"messages":[]
	}`)
	require.Equal(t, []string{"WebSearch"}, findDanglingToolRefs(body))
}

func TestFindDanglingToolRefs_IgnoresServerTools(t *testing.T) {
	// server tool（web_search 等）由上游内置，不需要在 tools 声明，不算悬空。
	body := []byte(`{
		"tools":[{"name":"Read","type":"custom"}],
		"messages":[{"role":"assistant","content":[
			{"type":"tool_use","id":"t1","name":"web_search","input":{}}]}]
	}`)
	require.Empty(t, findDanglingToolRefs(body))
}

func TestFindDanglingToolRefs_Dedup(t *testing.T) {
	// 同一悬空工具被多次引用只报一次。
	body := []byte(`{
		"tools":[{"name":"Read","type":"custom"}],
		"messages":[
			{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"WebSearch","input":{}}]},
			{"role":"assistant","content":[{"type":"tool_use","id":"t2","name":"WebSearch","input":{}}]}]
	}`)
	require.Equal(t, []string{"WebSearch"}, findDanglingToolRefs(body))
}

func TestFindDanglingToolRefs_NoTools(t *testing.T) {
	// 没有 tools 字段：tool_use 引用全部悬空（仍排除 server tool）。
	body := []byte(`{
		"messages":[{"role":"assistant","content":[
			{"type":"tool_use","id":"t1","name":"WebSearch","input":{}}]}]
	}`)
	require.Equal(t, []string{"WebSearch"}, findDanglingToolRefs(body))
}

func TestFindDanglingToolRefs_EmptyBody(t *testing.T) {
	require.Empty(t, findDanglingToolRefs([]byte{}))
	require.Empty(t, findDanglingToolRefs(nil))
}

func TestHasNewDangling(t *testing.T) {
	// after 引入了 before 没有的工具 → true（我们的改写新增了悬空）。
	require.True(t, hasNewDangling([]string{"A"}, []string{"A", "B"}))
	require.True(t, hasNewDangling(nil, []string{"B"}))
	// after 没有新增 → false（客户端原本就悬空，或已修复）。
	require.False(t, hasNewDangling([]string{"A"}, []string{"A"}))
	require.False(t, hasNewDangling([]string{"A", "B"}, []string{"A"}))
	require.False(t, hasNewDangling(nil, nil))
}
