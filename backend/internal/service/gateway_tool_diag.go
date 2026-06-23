package service

import (
	"sort"
	"strings"

	"github.com/tidwall/gjson"
)

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
