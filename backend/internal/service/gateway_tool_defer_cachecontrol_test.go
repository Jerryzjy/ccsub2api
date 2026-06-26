package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Anthropic 明确禁止"同一工具同时带 defer_loading=true 和 cache_control"
// （invalid_request_error）。客户端（Claude Code 对 MCP 工具）可能自带
// cache_control 到一个 defer_loading 工具上；网关必须在转发前剥离它，
// 否则上游返回 400:
//   "Tool 'X' cannot have both defer_loading=true and cache_control set."

// 客户端自带 cache_control 在 defer_loading 工具上 —— 必须被剥离。
func TestApplyToolsLastCacheBreakpoint_StripsCacheControlOnDeferLoading(t *testing.T) {
	body := []byte(`{"tools":[
		{"name":"bash"},
		{"name":"mcp_a","custom":{"defer_loading":true},"cache_control":{"type":"ephemeral"}}
	]}`)
	got := applyToolsLastCacheBreakpoint(body)

	require.False(t, gjson.GetBytes(got, `tools.1.cache_control`).Exists(),
		"defer_loading tool must NOT carry cache_control after processing")
	// 断点应回退到可缓存的 bash 上。
	require.Equal(t, "ephemeral", gjson.GetBytes(got, `tools.0.cache_control.type`).String(),
		"breakpoint must fall back to the last cacheable tool")
}

// 多个 defer_loading 工具都自带 cache_control —— 全部剥离。
func TestApplyToolsLastCacheBreakpoint_StripsAllDeferLoadingCacheControl(t *testing.T) {
	body := []byte(`{"tools":[
		{"name":"keep"},
		{"name":"mcp_a","custom":{"defer_loading":true},"cache_control":{"type":"ephemeral"}},
		{"name":"mcp_b","custom":{"defer_loading":true},"cache_control":{"type":"ephemeral","ttl":"1h"}}
	]}`)
	got := applyToolsLastCacheBreakpoint(body)

	require.False(t, gjson.GetBytes(got, `tools.1.cache_control`).Exists())
	require.False(t, gjson.GetBytes(got, `tools.2.cache_control`).Exists())
}

// 端到端：经过第三方伪装路径（排序 + 混淆 + 断点），defer_loading 工具
// 不得带 cache_control。复刻线上 "analyze_mcp15 / validate_mcp16" 报错场景。
func TestApplyThirdPartyToolMimicry_StripsDeferLoadingCacheControl(t *testing.T) {
	// 6 个工具触发动态混淆（> dynamicToolMapThreshold=5），其中两个 MCP 工具
	// defer_loading 且自带 cache_control。
	body := []byte(`{"tools":[
		{"name":"bash"},
		{"name":"edit"},
		{"name":"read"},
		{"name":"write"},
		{"name":"mcp_fifteen","custom":{"defer_loading":true},"cache_control":{"type":"ephemeral"}},
		{"name":"mcp_sixteen","custom":{"defer_loading":true},"cache_control":{"type":"ephemeral"}}
	]}`)
	got, _ := applyThirdPartyToolMimicry(body)

	tools := gjson.GetBytes(got, "tools").Array()
	for i, tool := range tools {
		if tool.Get("custom.defer_loading").Bool() {
			require.False(t, tool.Get("cache_control").Exists(),
				"tool index %d is defer_loading and must not have cache_control", i)
		}
	}
}
