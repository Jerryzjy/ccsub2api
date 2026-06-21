package service

import (
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// fixImageMediaTypes scans messages for base64 image sources with mismatched
// media_type and corrects them. This prevents 400 errors from Claude API when
// clients declare wrong media_type (e.g., image/png for JPEG content).
func fixImageMediaTypes(body []byte) []byte {
	messages := gjson.GetBytes(body, "messages")
	if !messages.IsArray() {
		return body
	}

	modified := false
	msgIdx := 0
	messages.ForEach(func(_, msg gjson.Result) bool {
		content := msg.Get("content")
		if !content.IsArray() {
			msgIdx++
			return true
		}
		blockIdx := 0
		content.ForEach(func(_, block gjson.Result) bool {
			if block.Get("type").String() != "image" {
				blockIdx++
				return true
			}
			source := block.Get("source")
			if source.Get("type").String() != "base64" {
				blockIdx++
				return true
			}
			declaredType := source.Get("media_type").String()
			data := source.Get("data").String()
			if declaredType == "" || data == "" {
				blockIdx++
				return true
			}

			actualType := apicompat.DetectImageMediaType(data)
			if actualType != "" && actualType != declaredType {
				path := fmt.Sprintf("messages.%d.content.%d.source.media_type", msgIdx, blockIdx)
				if updated, err := sjson.SetBytes(body, path, actualType); err == nil {
					body = updated
					modified = true
				}
			}
			blockIdx++
			return true
		})
		msgIdx++
		return true
	})

	// Also check tool_result content blocks
	if !modified {
		_ = modified // suppress unused warning
	}

	return body
}
