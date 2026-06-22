package service

import (
	"encoding/json"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// noPrefillModels lists models that do NOT support assistant prefill.
// When new models are released without prefill support, add them here.
var noPrefillModels = map[string]bool{
	"claude-opus-4-6":            true,
	"claude-opus-4-6-20260205":   true,
	"claude-opus-4-7":            true,
	"claude-opus-4-7-20260620":   true,
	"claude-opus-4-8":            true,
	"claude-sonnet-4-6":          true,
	"claude-sonnet-4-6-20260529": true,
}

// stripAssistantPrefillIfUnsupported removes the last assistant message if:
// 1. The model does not support assistant prefill
// 2. The last message role is "assistant"
// 3. The last message content is empty or whitespace-only
//
// This prevents 400 errors like:
// "This model does not support assistant message prefill. The conversation must end with a user message."
func stripAssistantPrefillIfUnsupported(body []byte, modelID string) ([]byte, error) {
	if !noPrefillModels[modelID] {
		return body, nil
	}

	// Parse messages array
	messagesResult := gjson.GetBytes(body, "messages")
	if !messagesResult.Exists() || !messagesResult.IsArray() {
		return body, nil
	}

	messages := messagesResult.Array()
	if len(messages) == 0 {
		return body, nil
	}

	// Check last message
	lastMsg := messages[len(messages)-1]
	role := lastMsg.Get("role").String()
	if role != "assistant" {
		return body, nil
	}

	// Check if content is empty
	content := lastMsg.Get("content")
	isEmpty := false

	switch content.Type {
	case gjson.String:
		isEmpty = content.String() == ""
	case gjson.JSON:
		if content.IsArray() {
			arr := content.Array()
			if len(arr) == 0 {
				isEmpty = true
			} else {
				allEmpty := true
				for _, block := range arr {
					if block.Get("type").String() == "text" {
						if block.Get("text").String() != "" {
							allEmpty = false
							break
						}
					}
				}
				isEmpty = allEmpty
			}
		}
	case gjson.Null:
		isEmpty = true
	}

	if !isEmpty {
		return body, nil
	}

	// Remove the last message
	newMessages := make([]json.RawMessage, len(messages)-1)
	for i := 0; i < len(messages)-1; i++ {
		newMessages[i] = json.RawMessage(messages[i].Raw)
	}

	newBody, err := sjson.SetBytes(body, "messages", newMessages)
	if err != nil {
		return body, err
	}

	return newBody, nil
}
