package service

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tidwall/gjson"
)

// imageDiagMaxValueLen 诊断日志中单个字段值的截断长度，避免把 base64/url 打满日志。
const imageDiagMaxValueLen = 64

// isMissingImageSourceError 判断上游 400 错误消息是否为"image 块缺 source"类。
// 小写不敏感匹配 image + source + field required，避免误命中 thinking.signature 等其它 400。
func isMissingImageSourceError(msg string) bool {
	m := strings.ToLower(msg)
	return strings.Contains(m, "image") &&
		strings.Contains(m, "source") &&
		strings.Contains(m, "field required")
}

// diagnoseMissingImageSource 遍历 body 的 messages[].content[]（含 tool_result 嵌套 content[]），
// 找出 type=="image" 但缺合法 source 的块，返回其脱敏结构骨架列表用于日志。
// 只读，不修改 body。脱敏：仅保留 key 名与 type，长字符串值截断到 imageDiagMaxValueLen。
func diagnoseMissingImageSource(body []byte) []string {
	if len(body) == 0 {
		return nil
	}
	var out []string
	messages := gjson.GetBytes(body, "messages")
	if !messages.IsArray() {
		return nil
	}
	messages.ForEach(func(mi, msg gjson.Result) bool {
		content := msg.Get("content")
		if !content.IsArray() {
			return true
		}
		content.ForEach(func(bi, block gjson.Result) bool {
			blockType := block.Get("type").String()
			if blockType == "image" {
				if !hasValidImageSource(block) {
					out = append(out, formatImageOffender(
						fmt.Sprintf("msg[%d].content[%d]", mi.Int(), bi.Int()), block))
				}
				return true
			}
			if blockType == "tool_result" {
				nested := block.Get("content")
				if nested.IsArray() {
					nested.ForEach(func(ni, sub gjson.Result) bool {
						if sub.Get("type").String() == "image" && !hasValidImageSource(sub) {
							out = append(out, formatImageOffender(
								fmt.Sprintf("msg[%d].content[%d].tool_result.content[%d]", mi.Int(), bi.Int(), ni.Int()), sub))
						}
						return true
					})
				}
			}
			return true
		})
		return true
	})
	return out
}

// hasValidImageSource 判断 image 块是否带合法 source（对象形态）。
func hasValidImageSource(block gjson.Result) bool {
	src := block.Get("source")
	return src.Exists() && src.IsObject()
}

// formatImageOffender 把畸形 image 块输出为脱敏结构骨架。
func formatImageOffender(path string, block gjson.Result) string {
	keys := make([]string, 0, 4)
	pairs := make([]string, 0, 4)
	block.ForEach(func(k, v gjson.Result) bool {
		key := k.String()
		keys = append(keys, key)
		// 仅对标量值给出截断后的内容，对象/数组只标类型，避免泄露完整数据。
		switch {
		case v.IsObject():
			pairs = append(pairs, key+":{object}")
		case v.IsArray():
			pairs = append(pairs, key+":[array]")
		default:
			pairs = append(pairs, key+":"+truncateDiagValue(v.String()))
		}
		return true
	})
	sort.Strings(keys)
	return fmt.Sprintf("%s = {keys:[%s], %s}", path, strings.Join(keys, ","), strings.Join(pairs, ", "))
}

// truncateDiagValue 截断长字符串值，避免把 base64/url 打满日志。
func truncateDiagValue(s string) string {
	if len(s) <= imageDiagMaxValueLen {
		return s
	}
	return s[:imageDiagMaxValueLen] + "...(truncated)"
}
