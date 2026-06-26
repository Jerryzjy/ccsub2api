package service

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// backfillDanglingToolDeclarations 兜底修复"被 tool_use / tool_choice 引用、但没在
// tools[] 声明"的悬空工具引用：为每个悬空工具名补一个最小 schema 的占位声明，
// 使请求自洽，避免上游返回
// "Tool reference 'X' not found in available tools"（invalid_request_error）。
//
// 悬空根因通常在客户端/中转层（跨轮次发送不一致的工具集），与本仓的工具名混淆无关，
// 且混淆路径和透传路径都会中招——故此兜底应在所有 OAuth 转发路径上、在工具名混淆
// 之前调用，保证补进的真名也能被后续混淆一并改写、保持一致。
//
// 占位 schema 用 {"type":"object"}（Anthropic 接受的最宽松输入约束）。无悬空时返回
// 原 body 不变。
func backfillDanglingToolDeclarations(body []byte) []byte {
	if len(body) == 0 {
		return body
	}
	dangling := findDanglingToolRefs(body)
	if len(dangling) == 0 {
		return body
	}

	out := body
	for _, name := range dangling {
		decl := fmt.Sprintf(`{"name":%q,"input_schema":{"type":"object"}}`, name)
		if next, err := sjson.SetRawBytes(out, "tools.-1", []byte(decl)); err == nil {
			out = next
		}
	}
	return out
}

// findDanglingToolRefs 扫描 Anthropic /v1/messages body，找出"被 tool_use /
// tool_choice 引用、但没有在 tools 数组声明"的工具名（悬空引用）。
//
// 用途：诊断上游 "Tool reference 'X' not found in available tools" 错误。
// 该错误由 Anthropic 在请求自相矛盾时返回——历史消息里的 tool_use 引用了某工具，
// 但本轮 tools 没声明它。第三方客户端跨轮次发送不一致的工具集是常见诱因。
//
// 纯只读：不修改 body。返回去重 + 字典序排序的悬空工具名列表（无则返回 nil）。
//
// server tool（web_search / computer / bash 等）由上游内置，本就不需要在 tools
// 声明，所以从悬空集中排除，避免噪音误报。
func findDanglingToolRefs(body []byte) []string {
	if len(body) == 0 {
		return nil
	}

	declared := make(map[string]struct{})
	if tools := gjson.GetBytes(body, "tools"); tools.IsArray() {
		tools.ForEach(func(_, t gjson.Result) bool {
			if name := t.Get("name").String(); name != "" {
				declared[name] = struct{}{}
			}
			return true
		})
	}

	dangling := make(map[string]struct{})

	consider := func(name string) {
		if name == "" {
			return
		}
		if _, ok := declared[name]; ok {
			return
		}
		if isServerToolName(name) {
			return
		}
		dangling[name] = struct{}{}
	}

	if tc := gjson.GetBytes(body, "tool_choice"); tc.Get("type").String() == "tool" {
		consider(tc.Get("name").String())
	}

	if messages := gjson.GetBytes(body, "messages"); messages.IsArray() {
		messages.ForEach(func(_, msg gjson.Result) bool {
			content := msg.Get("content")
			if !content.IsArray() {
				return true
			}
			content.ForEach(func(_, blk gjson.Result) bool {
				if blk.Get("type").String() == "tool_use" {
					consider(blk.Get("name").String())
				}
				return true
			})
			return true
		})
	}

	if len(dangling) == 0 {
		return nil
	}
	out := make([]string, 0, len(dangling))
	for name := range dangling {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// isServerToolName 判断一个工具名是否为 Anthropic 内置 server tool。
// server tool 由上游提供，不需要在 tools 声明，因此不算悬空引用。
func isServerToolName(name string) bool {
	switch name {
	case toolNameWebSearch, toolNameGoogleSearch, toolNameWebSearch2025:
		return true
	}
	// 带版本后缀的 server tool：computer_20250124 / bash_20250124 /
	// text_editor_20250728 / code_execution_20250522 等。
	for _, prefix := range serverToolNamePrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// serverToolNamePrefixes 是 Anthropic server tool 名的已知前缀。
var serverToolNamePrefixes = []string{
	"web_search",
	"computer",
	"bash_",
	"text_editor",
	"code_execution",
}
