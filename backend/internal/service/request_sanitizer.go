package service

import (
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

var maxOutputTokensByModel = map[string]int{
	"claude-opus-4-6":            128000,
	"claude-opus-4-6-20260205":   128000,
	"claude-opus-4-7":            128000,
	"claude-opus-4-7-20260620":   128000,
	"claude-sonnet-4-6":          128000,
	"claude-sonnet-4-6-20260529": 128000,
	"claude-sonnet-4-5":          16384,
	"claude-sonnet-4-5-20250514": 16384,
	"claude-haiku-4-5":           8192,
	"claude-haiku-4-5-20251001":  8192,
}

// sanitizeRequestParams fixes common request parameter errors before sending upstream.
func sanitizeRequestParams(body []byte, modelID string) []byte {
	body = clampMaxTokens(body, modelID)
	body = ensureMinBudgetTokens(body)
	body = fixToolTypeFunction(body)
	body = stripUnsupportedServerTools(body, modelID)
	body = fixToolResultImageMediaTypes(body)
	body = fixImageURLToImage(body)
	body = stripThinkingIncompatibleParams(body)
	return body
}

func clampMaxTokens(body []byte, modelID string) []byte {
	maxTokens := gjson.GetBytes(body, "max_tokens").Int()
	if maxTokens <= 0 {
		return body
	}
	limit := resolveMaxOutputTokens(modelID)
	if limit <= 0 || maxTokens <= int64(limit) {
		return body
	}
	if updated, err := sjson.SetBytes(body, "max_tokens", limit); err == nil {
		return updated
	}
	return body
}

func resolveMaxOutputTokens(modelID string) int {
	if limit, ok := maxOutputTokensByModel[modelID]; ok {
		return limit
	}
	for prefix, limit := range maxOutputTokensByModel {
		if strings.HasPrefix(modelID, prefix) {
			return limit
		}
	}
	return 0
}

func ensureMinBudgetTokens(body []byte) []byte {
	thinkingType := gjson.GetBytes(body, "thinking.type").String()
	if thinkingType != "enabled" && thinkingType != "adaptive" {
		return body
	}
	budget := gjson.GetBytes(body, "thinking.budget_tokens")
	if !budget.Exists() || budget.Int() >= 1024 {
		return body
	}
	if updated, err := sjson.SetBytes(body, "thinking.budget_tokens", 1024); err == nil {
		return updated
	}
	return body
}

// fixToolTypeFunction replaces "type":"function" with "type":"custom" in tools.
// Claude API only accepts "custom" or built-in types (bash_20250124, web_search_20250305, etc.).
// "function" is an OpenAI convention that clients sometimes send to /v1/messages directly.
func fixToolTypeFunction(body []byte) []byte {
	tools := gjson.GetBytes(body, "tools")
	if !tools.IsArray() {
		return body
	}
	modified := false
	idx := 0
	tools.ForEach(func(_, tool gjson.Result) bool {
		toolType := tool.Get("type").String()
		if toolType == "function" {
			path := fmt.Sprintf("tools.%d.type", idx)
			if updated, err := sjson.SetBytes(body, path, "custom"); err == nil {
				body = updated
				modified = true
			}
		}
		idx++
		return true
	})
	_ = modified
	return body
}

func stripUnsupportedServerTools(body []byte, modelID string) []byte {
	if strings.Contains(modelID, "claude") {
		return body
	}
	tools := gjson.GetBytes(body, "tools")
	if !tools.IsArray() {
		return body
	}
	hasServerTools := false
	tools.ForEach(func(_, tool gjson.Result) bool {
		t := tool.Get("type").String()
		if t == "web_search_20250305" || t == "computer_20250124" || t == "text_editor_20250124" {
			hasServerTools = true
			return false
		}
		return true
	})
	if !hasServerTools {
		return body
	}
	var parts []string
	tools.ForEach(func(_, tool gjson.Result) bool {
		t := tool.Get("type").String()
		if t != "web_search_20250305" && t != "computer_20250124" && t != "text_editor_20250124" {
			parts = append(parts, tool.Raw)
		}
		return true
	})
	if len(parts) == 0 {
		if updated, err := sjson.DeleteBytes(body, "tools"); err == nil {
			return updated
		}
		return body
	}
	raw := "[" + strings.Join(parts, ",") + "]"
	if updated, err := sjson.SetRawBytes(body, "tools", []byte(raw)); err == nil {
		return updated
	}
	return body
}

func fixToolResultImageMediaTypes(body []byte) []byte {
	messages := gjson.GetBytes(body, "messages")
	if !messages.IsArray() {
		return body
	}
	msgIdx := 0
	messages.ForEach(func(_, msg gjson.Result) bool {
		content := msg.Get("content")
		if !content.IsArray() {
			msgIdx++
			return true
		}
		blockIdx := 0
		content.ForEach(func(_, block gjson.Result) bool {
			if block.Get("type").String() == "tool_result" {
				nested := block.Get("content")
				if nested.IsArray() {
					nestedIdx := 0
					nested.ForEach(func(_, n gjson.Result) bool {
						if n.Get("type").String() == "image" && n.Get("source.type").String() == "base64" {
							declared := n.Get("source.media_type").String()
							data := n.Get("source.data").String()
							if declared != "" && data != "" {
								actual := apicompat.DetectImageMediaType(data)
								if actual != "" && actual != declared {
									path := fmt.Sprintf("messages.%d.content.%d.content.%d.source.media_type", msgIdx, blockIdx, nestedIdx)
									if updated, err := sjson.SetBytes(body, path, actual); err == nil {
										body = updated
									}
								}
							}
						}
						nestedIdx++
						return true
					})
				}
			}
			blockIdx++
			return true
		})
		msgIdx++
		return true
	})
	return body
}

// fixImageURLToImage rewrites "type":"image_url" to "type":"image" in all
// content blocks (messages and tool_result nested content).
// Claude API expects "image" type, but OpenAI-style clients send "image_url".
func fixImageURLToImage(body []byte) []byte {
	messages := gjson.GetBytes(body, "messages")
	if !messages.IsArray() {
		return body
	}
	msgIdx := 0
	messages.ForEach(func(_, msg gjson.Result) bool {
		content := msg.Get("content")
		if !content.IsArray() {
			msgIdx++
			return true
		}
		blockIdx := 0
		content.ForEach(func(_, block gjson.Result) bool {
			blockType := block.Get("type").String()
			// Direct image_url block in message content
			if blockType == "image_url" {
				path := fmt.Sprintf("messages.%d.content.%d.type", msgIdx, blockIdx)
				if updated, err := sjson.SetBytes(body, path, "image"); err == nil {
					body = updated
				}
			}
			// Nested content inside tool_result
			if blockType == "tool_result" {
				nested := block.Get("content")
				if nested.IsArray() {
					nestedIdx := 0
					nested.ForEach(func(_, n gjson.Result) bool {
						if n.Get("type").String() == "image_url" {
							path := fmt.Sprintf("messages.%d.content.%d.content.%d.type", msgIdx, blockIdx, nestedIdx)
							if updated, err := sjson.SetBytes(body, path, "image"); err == nil {
								body = updated
							}
						}
						nestedIdx++
						return true
					})
				}
			}
			blockIdx++
			return true
		})
		msgIdx++
		return true
	})
	return body
}

// stripThinkingIncompatibleParams removes parameters that are forbidden
// when thinking is enabled: top_k and top_p.
// Claude API: "top_k/top_p must be unset when thinking is enabled"
func stripThinkingIncompatibleParams(body []byte) []byte {
	thinkingType := gjson.GetBytes(body, "thinking.type").String()
	if thinkingType != "enabled" && thinkingType != "adaptive" {
		return body
	}
	if gjson.GetBytes(body, "top_k").Exists() {
		if updated, err := sjson.DeleteBytes(body, "top_k"); err == nil {
			body = updated
		}
	}
	if gjson.GetBytes(body, "top_p").Exists() {
		if updated, err := sjson.DeleteBytes(body, "top_p"); err == nil {
			body = updated
		}
	}
	return body
}
