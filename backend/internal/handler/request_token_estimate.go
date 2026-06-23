package handler

import (
	"unicode/utf8"

	"github.com/tidwall/gjson"
)

// estimateAnthropicTokens 基于请求体的 system + messages + tools 内容字符数 ÷4 估算 token 数。
//
// 纯本地、零网络开销。用于"防极端值"的超长拦截：宁可略微高估也不能漏算，否则
// 必然超限的请求会绕过预拦截、打到上游被真实 tokenizer 判 "prompt is too long"。
//
// 统计范围（覆盖真实大请求的 token 主要来源）：
//   - system：string 或 []block 的 text
//   - messages[*].content：text 块的 text；tool_use 块的 input；tool_result 块的
//     content（无论 string 还是 []block，按原始 JSON 字符计）；image / document 等
//     按尺寸/页数计费的块忽略，避免字符估算误伤
//   - tools：整个 tools 数组的原始 JSON（schema 在大工具集下也占可观 token）
//
// 对中文偏保守（估值偏大）。tool_result（读取的大文件 / 命令输出）通常是超长请求的
// 最大头，早期版本只数 text 块导致严重低估，这里必须纳入。
func estimateAnthropicTokens(body []byte) int64 {
	if len(body) == 0 {
		return 0
	}

	var chars int64

	system := gjson.GetBytes(body, "system")
	chars += countAnthropicTextChars(system)

	gjson.GetBytes(body, "messages").ForEach(func(_, msg gjson.Result) bool {
		chars += countMessageContentChars(msg.Get("content"))
		return true
	})

	// tools schema：大工具集（Claude Code 默认带数十个工具）的 schema 字符也计入。
	if tools := gjson.GetBytes(body, "tools"); tools.IsArray() {
		chars += int64(utf8.RuneCountInString(tools.Raw))
	}

	return chars / 4
}

// countMessageContentChars 统计单条 message 的 content 值里的字符数。
//
// content 兼容 string 与 []block 两种形态。对 []block：
//   - text 块只计 text 字段
//   - tool_use 块计 input 的原始 JSON（工具调用参数可能很大）
//   - tool_result 块计 content 的原始 JSON（工具返回的文件/命令输出，是超长请求的最大头）
//   - 其余块（image / document 等）忽略：它们按图片尺寸/页数而非字符计费，
//     用字符数估算会严重高估，反而误伤正常请求
func countMessageContentChars(v gjson.Result) int64 {
	if v.Type == gjson.String {
		return int64(utf8.RuneCountInString(v.String()))
	}
	if !v.IsArray() {
		return 0
	}
	var chars int64
	v.ForEach(func(_, block gjson.Result) bool {
		switch block.Get("type").String() {
		case "text":
			chars += int64(utf8.RuneCountInString(block.Get("text").String()))
		case "tool_use":
			chars += int64(utf8.RuneCountInString(block.Get("input").Raw))
		case "tool_result":
			chars += int64(utf8.RuneCountInString(block.Get("content").Raw))
		}
		return true
	})
	return chars
}

// countAnthropicTextChars 统计一个 system 值里的文本 rune 数。
// 兼容 string 与 []block（取 type==text 的 text）两种形态。
func countAnthropicTextChars(v gjson.Result) int64 {
	if v.Type == gjson.String {
		return int64(utf8.RuneCountInString(v.String()))
	}
	if !v.IsArray() {
		return 0
	}
	var chars int64
	v.ForEach(func(_, block gjson.Result) bool {
		if block.Get("type").String() == "text" {
			chars += int64(utf8.RuneCountInString(block.Get("text").String()))
		}
		return true
	})
	return chars
}
