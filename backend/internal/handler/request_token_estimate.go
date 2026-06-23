package handler

import (
	"unicode/utf8"

	"github.com/tidwall/gjson"
)

// estimateAnthropicTokens 基于请求体的 system + messages 文本字符数 ÷4 估算 token 数。
//
// 纯本地、零网络开销。只统计真正的文本内容（不含 tools schema / 元数据），
// 按 rune 计数后 ÷4。对中文偏保守（估值偏大），用于"防极端值"的超长拦截正合适。
func estimateAnthropicTokens(body []byte) int64 {
	if len(body) == 0 {
		return 0
	}

	var chars int64

	system := gjson.GetBytes(body, "system")
	chars += countAnthropicTextChars(system)

	gjson.GetBytes(body, "messages").ForEach(func(_, msg gjson.Result) bool {
		chars += countAnthropicTextChars(msg.Get("content"))
		return true
	})

	return chars / 4
}

// countAnthropicTextChars 统计一个 content/system 值里的文本 rune 数。
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
