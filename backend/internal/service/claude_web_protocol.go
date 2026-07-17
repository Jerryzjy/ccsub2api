package service

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

var ErrClaudeWebUnsupportedContent = errors.New("claude web session supports text content only")

type ClaudeWebTurnMessageUUIDs struct {
	HumanMessageUUID     string `json:"human_message_uuid"`
	AssistantMessageUUID string `json:"assistant_message_uuid"`
}

type ClaudeWebCreateConversationParams struct {
	Name                           string `json:"name"`
	Model                          string `json:"model"`
	IncludeConversationPreferences bool   `json:"include_conversation_preferences"`
	PaprikaMode                    any    `json:"paprika_mode"`
	CompassMode                    any    `json:"compass_mode"`
	ToolSearchMode                 string `json:"tool_search_mode"`
	IsTemporary                    bool   `json:"is_temporary"`
	EnabledImagine                 bool   `json:"enabled_imagine"`
}

type ClaudeWebCompletionRequest struct {
	Prompt                   string                            `json:"prompt"`
	ParentMessageUUID        string                            `json:"parent_message_uuid"`
	Timezone                 string                            `json:"timezone"`
	Locale                   string                            `json:"locale"`
	Model                    string                            `json:"model"`
	ThinkingMode             string                            `json:"thinking_mode"`
	TurnMessageUUIDs         ClaudeWebTurnMessageUUIDs         `json:"turn_message_uuids"`
	Attachments              []any                             `json:"attachments"`
	Files                    []any                             `json:"files"`
	SyncSources              []any                             `json:"sync_sources"`
	RenderingMode            string                            `json:"rendering_mode"`
	CreateConversationParams ClaudeWebCreateConversationParams `json:"create_conversation_params"`
}

type claudeWebAnthropicRequest struct {
	System   json.RawMessage             `json:"system"`
	Messages []claudeWebAnthropicMessage `json:"messages"`
}

type claudeWebAnthropicMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type claudeWebContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func ResolveClaudeWebModel(model string) string {
	normalized := strings.ToLower(strings.TrimSpace(model))
	switch normalized {
	case "claude-fable-5", "claude-opus-4-8", "claude-haiku-4-5",
		"claude-opus-4-7", "claude-opus-4-6", "claude-opus-3",
		"claude-sonnet-4-6", "claude-sonnet-5":
		return normalized
	}
	switch {
	case strings.Contains(normalized, "haiku"):
		return "claude-haiku-4-5"
	case strings.Contains(normalized, "opus"):
		return "claude-opus-4-8"
	case strings.Contains(normalized, "fable"):
		return "claude-fable-5"
	default:
		return "claude-sonnet-5"
	}
}

func BuildClaudeWebPrompt(body []byte) (string, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var request claudeWebAnthropicRequest
	if err := decoder.Decode(&request); err != nil {
		return "", fmt.Errorf("decode anthropic request: %w", err)
	}
	if len(request.Messages) == 0 {
		return "", errors.New("messages is required")
	}

	parts := make([]string, 0, len(request.Messages)+2)
	if len(bytes.TrimSpace(request.System)) > 0 && string(bytes.TrimSpace(request.System)) != "null" {
		system, err := claudeWebContentText(request.System)
		if err != nil {
			return "", fmt.Errorf("system: %w", err)
		}
		if system != "" {
			parts = append(parts, "[System]\n"+system)
		}
	}

	for i, message := range request.Messages {
		text, err := claudeWebContentText(message.Content)
		if err != nil {
			return "", fmt.Errorf("messages[%d]: %w", i, err)
		}
		switch message.Role {
		case "user":
			parts = append(parts, "[Human]\n"+text)
		case "assistant":
			parts = append(parts, "[Assistant]\n"+text)
		default:
			return "", fmt.Errorf("messages[%d]: unsupported role", i)
		}
	}
	parts = append(parts, "[Assistant]\n")
	return strings.Join(parts, "\n\n"), nil
}

func claudeWebContentText(raw json.RawMessage) (string, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "", nil
	}
	if raw[0] == '"' {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return "", errors.New("invalid text content")
		}
		return text, nil
	}
	if raw[0] != '[' {
		return "", ErrClaudeWebUnsupportedContent
	}
	var blocks []claudeWebContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return "", errors.New("invalid content blocks")
	}
	var text strings.Builder
	for _, block := range blocks {
		if block.Type != "text" {
			return "", ErrClaudeWebUnsupportedContent
		}
		text.WriteString(block.Text)
	}
	return text.String(), nil
}

func BuildClaudeWebCompletion(body []byte, model string) (ClaudeWebCompletionRequest, error) {
	prompt, err := BuildClaudeWebPrompt(body)
	if err != nil {
		return ClaudeWebCompletionRequest{}, err
	}
	if strings.TrimSpace(model) == "" {
		return ClaudeWebCompletionRequest{}, errors.New("model is required")
	}
	return ClaudeWebCompletionRequest{
		Prompt:            prompt,
		ParentMessageUUID: uuid.NewString(),
		Timezone:          "Asia/Shanghai",
		Locale:            "en-US",
		Model:             model,
		ThinkingMode:      "auto",
		TurnMessageUUIDs: ClaudeWebTurnMessageUUIDs{
			HumanMessageUUID:     uuid.NewString(),
			AssistantMessageUUID: uuid.NewString(),
		},
		Attachments:   []any{},
		Files:         []any{},
		SyncSources:   []any{},
		RenderingMode: "messages",
		CreateConversationParams: ClaudeWebCreateConversationParams{
			Model:                          model,
			IncludeConversationPreferences: true,
			ToolSearchMode:                 "auto",
		},
	}, nil
}

type ClaudeWebStreamMeta struct {
	Model       string
	MessageID   string
	InputTokens int
}

type claudeWebStreamPayload struct {
	Type       string `json:"type"`
	Completion string `json:"completion"`
	Text       string `json:"text"`
	Delta      struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"delta"`
}

func TranslateClaudeWebSSE(src io.Reader, dst io.Writer, meta ClaudeWebStreamMeta) (ClaudeUsage, error) {
	if strings.TrimSpace(meta.MessageID) == "" {
		meta.MessageID = "msg_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	}
	usage := ClaudeUsage{InputTokens: meta.InputTokens}
	if err := writeClaudeWebSSE(dst, "message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id": meta.MessageID, "type": "message", "role": "assistant",
			"content": []any{}, "model": meta.Model,
			"stop_reason": nil, "stop_sequence": nil,
			"usage": map[string]int{"input_tokens": usage.InputTokens, "output_tokens": 0},
		},
	}); err != nil {
		return usage, err
	}
	if err := writeClaudeWebSSE(dst, "content_block_start", map[string]any{
		"type": "content_block_start", "index": 0,
		"content_block": map[string]string{"type": "text", "text": ""},
	}); err != nil {
		return usage, err
	}

	outputRunes := 0
	err := scanClaudeWebSSE(src, func(event string, data []byte) error {
		if event == "error" {
			return errors.New("Claude Web stream error")
		}
		var payload claudeWebStreamPayload
		if err := json.Unmarshal(data, &payload); err != nil {
			return errors.New("decode Claude Web stream event")
		}
		if payload.Type == "error" {
			return errors.New("Claude Web stream error")
		}
		text := payload.Delta.Text
		if text == "" {
			text = payload.Completion
		}
		if text == "" && (event == "text_delta" || payload.Type == "text_delta") {
			text = payload.Text
		}
		if text == "" {
			return nil
		}
		outputRunes += utf8.RuneCountInString(text)
		return writeClaudeWebSSE(dst, "content_block_delta", map[string]any{
			"type": "content_block_delta", "index": 0,
			"delta": map[string]string{"type": "text_delta", "text": text},
		})
	})
	if err != nil {
		return usage, err
	}
	usage.OutputTokens = estimateClaudeWebTokens(outputRunes)
	if err := writeClaudeWebSSE(dst, "content_block_stop", map[string]any{
		"type": "content_block_stop", "index": 0,
	}); err != nil {
		return usage, err
	}
	if err := writeClaudeWebSSE(dst, "message_delta", map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": "end_turn", "stop_sequence": nil},
		"usage": map[string]int{"output_tokens": usage.OutputTokens},
	}); err != nil {
		return usage, err
	}
	if err := writeClaudeWebSSE(dst, "message_stop", map[string]string{"type": "message_stop"}); err != nil {
		return usage, err
	}
	return usage, nil
}

func AggregateClaudeWebSSE(src io.Reader, model, messageID string, inputTokens int) ([]byte, ClaudeUsage, error) {
	if strings.TrimSpace(messageID) == "" {
		messageID = "msg_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	}
	var text strings.Builder
	err := scanClaudeWebSSE(src, func(event string, data []byte) error {
		if event == "error" {
			return errors.New("Claude Web stream error")
		}
		var payload claudeWebStreamPayload
		if err := json.Unmarshal(data, &payload); err != nil {
			return errors.New("decode Claude Web stream event")
		}
		if payload.Type == "error" {
			return errors.New("Claude Web stream error")
		}
		delta := payload.Delta.Text
		if delta == "" {
			delta = payload.Completion
		}
		if delta == "" && (event == "text_delta" || payload.Type == "text_delta") {
			delta = payload.Text
		}
		text.WriteString(delta)
		return nil
	})
	usage := ClaudeUsage{
		InputTokens:  inputTokens,
		OutputTokens: estimateClaudeWebTokens(utf8.RuneCountInString(text.String())),
	}
	if err != nil {
		return nil, usage, err
	}
	body, err := json.Marshal(map[string]any{
		"id": messageID, "type": "message", "role": "assistant", "model": model,
		"content":     []map[string]string{{"type": "text", "text": text.String()}},
		"stop_reason": "end_turn", "stop_sequence": nil,
		"usage": map[string]int{"input_tokens": usage.InputTokens, "output_tokens": usage.OutputTokens},
	})
	if err != nil {
		return nil, usage, errors.New("encode Claude Web response")
	}
	return body, usage, nil
}

func scanClaudeWebSSE(src io.Reader, handle func(event string, data []byte) error) error {
	scanner := bufio.NewScanner(src)
	buffer := make([]byte, 64<<10)
	scanner.Buffer(buffer, 4<<20)
	event := ""
	dataLines := make([]string, 0, 1)
	flush := func() error {
		if len(dataLines) == 0 {
			event = ""
			return nil
		}
		data := []byte(strings.Join(dataLines, "\n"))
		dataLines = dataLines[:0]
		currentEvent := event
		event = ""
		return handle(currentEvent, data)
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := flush(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if value, ok := strings.CutPrefix(line, "event:"); ok {
			event = strings.TrimSpace(value)
			continue
		}
		if value, ok := strings.CutPrefix(line, "data:"); ok {
			dataLines = append(dataLines, strings.TrimSpace(value))
		}
	}
	if err := scanner.Err(); err != nil {
		return errors.New("read Claude Web stream")
	}
	return flush()
}

func writeClaudeWebSSE(dst io.Writer, event string, data any) error {
	encoded, err := json.Marshal(data)
	if err != nil {
		return errors.New("encode Claude Web stream event")
	}
	_, err = fmt.Fprintf(dst, "event: %s\ndata: %s\n\n", event, encoded)
	return err
}

func estimateClaudeWebTokens(runes int) int {
	if runes <= 0 {
		return 0
	}
	estimate := runes / 4
	if estimate == 0 {
		return 1
	}
	return estimate
}
