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

// stripAssistantPrefillIfUnsupported removes trailing assistant messages when
// the target model does not support assistant prefill.
//
// Models like claude-opus-4-8 reject any conversation that ends with an
// assistant message (whether the prefill content is empty or not) with:
// "This model does not support assistant message prefill. The conversation must end with a user message."
//
// Real Claude Code never sends an assistant prefill — only third-party clients
// do. Since these models cannot continue a prefilled assistant turn, the only
// way to make the request succeed is to drop the trailing assistant message(s)
// so the conversation ends with a user message. The model then answers the
// preceding user turn normally. This is lossy by necessity: there is no
// faithful way to honor a prefill on a model that does not support it, and
// rewriting the prefill into a user turn would change the output contract
// (the model would return a full reply instead of a continuation), producing
// duplicated/garbled output for clients that follow the prefill protocol.
//
// Trailing assistant messages are removed in a loop to cover the edge case of
// several consecutive assistant turns, guaranteeing the result ends with a
// user message.
func stripAssistantPrefillIfUnsupported(body []byte, modelID string) ([]byte, error) {
	if !noPrefillModels[modelID] {
		return body, nil
	}

	messagesResult := gjson.GetBytes(body, "messages")
	if !messagesResult.Exists() || !messagesResult.IsArray() {
		return body, nil
	}

	messages := messagesResult.Array()
	if len(messages) == 0 {
		return body, nil
	}

	// Walk back from the tail, dropping consecutive assistant messages until the
	// conversation ends with a non-assistant (user) message.
	keep := len(messages)
	for keep > 0 && messages[keep-1].Get("role").String() == "assistant" {
		keep--
	}

	// Already ends with a user (or non-assistant) message — nothing to do.
	if keep == len(messages) {
		return body, nil
	}

	// keep == 0 means every message is an assistant turn — a malformed request
	// that has no valid prefix to preserve. Emptying messages lets the upstream
	// return a clearer "messages must not be empty" error than the prefill 400.
	newMessages := make([]json.RawMessage, keep)
	for i := 0; i < keep; i++ {
		newMessages[i] = json.RawMessage(messages[i].Raw)
	}

	newBody, err := sjson.SetBytes(body, "messages", newMessages)
	if err != nil {
		return body, err
	}

	return newBody, nil
}
