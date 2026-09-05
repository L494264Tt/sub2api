package securityaudit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	conversationKindDialogue  = "conversation"
	conversationKindAuxiliary = "auxiliary"
	conversationKindUnknown   = "unknown"
	conversationMessageLimit  = 256
	securityMonitorSignature  = "you are a security monitor for autonomous ai coding agents."
)

// Parse roles and typed blocks before applying any storage limit. System and
// tool content cannot consume the dialogue budget, even when sent first.
func captureConversationMessages(request Request, response string, limit int) (ConversationRequestDetails, string, int, bool, error) {
	var root map[string]any
	if err := json.Unmarshal(request.Body, &root); err != nil {
		return ConversationRequestDetails{}, "", 0, false, errors.New("conversation request JSON is invalid")
	}
	if limit < 1 || limit > MaxConversationCaptureRunes {
		limit = DefaultConversationRequestMaxRunes
	}
	details := ConversationRequestDetails{Version: 1, CurrentMessagePosition: -1, Messages: []ConversationMessage{}, Context: []ConversationMessage{}}
	position := 0
	monitor := false
	add := func(role, kind, text string) {
		if strings.TrimSpace(text) == "" {
			return
		}
		text = strings.ReplaceAll(text, "\x00", "")
		message := ConversationMessage{Role: role, Kind: kind, Content: text, Position: position}
		position++
		if role == "system" && strings.HasPrefix(strings.ToLower(strings.TrimSpace(text)), securityMonitorSignature) {
			monitor = true
		}
		if kind == "text" && (role == "user" || role == "assistant" || role == "model") {
			if role == "model" {
				message.Role = "assistant"
			}
			if role == "user" {
				details.CurrentMessagePosition = message.Position
			}
			details.Messages = append(details.Messages, message)
		} else {
			details.Context = append(details.Context, message)
		}
	}
	blocks := func(role string, content any) {
		var texts []string
		flush := func() {
			if len(texts) > 0 {
				add(role, "text", strings.Join(texts, "\n"))
				texts = nil
			}
		}
		switch content := content.(type) {
		case string:
			add(role, "text", content)
		case []any:
			for _, item := range content {
				block, ok := item.(map[string]any)
				if !ok {
					continue
				}
				typeName := jsonString(block, "type")
				text, hasText := block["text"].(string)
				if hasText && (typeName == "" || typeName == "text" || typeName == "input_text" || typeName == "output_text") {
					texts = append(texts, text)
					continue
				}
				switch {
				case typeName == "tool_use" || typeName == "tool_result":
					add(role, typeName, conversationJSONText(block))
				case block["functionCall"] != nil:
					add(role, "tool_use", conversationJSONText(block["functionCall"]))
				case block["functionResponse"] != nil:
					add(role, "tool_result", conversationJSONText(block["functionResponse"]))
				case typeName == "thinking" || typeName == "redacted_thinking":
					add(role, "reasoning", conversationJSONText(block))
				default:
					add(role, "attachment", "[attachment content not archived]")
				}
			}
			flush()
		}
	}
	blocks("system", root["system"])
	blocks("system", root["instructions"])
	for _, key := range []string{"systemInstruction", "system_instruction"} {
		if system, ok := root[key].(map[string]any); ok {
			blocks("system", system["parts"])
		}
	}
	if messages, ok := root["messages"].([]any); ok {
		for _, item := range messages {
			message, _ := item.(map[string]any)
			role := jsonString(message, "role")
			if role == "" {
				role = "user"
			}
			blocks(role, message["content"])
			if calls := message["tool_calls"]; calls != nil {
				add(role, "tool_use", conversationJSONText(calls))
			}
			if call := message["function_call"]; call != nil {
				add(role, "tool_use", conversationJSONText(call))
			}
		}
	}
	switch input := root["input"].(type) {
	case string:
		blocks("user", input)
	case []any:
		for _, item := range input {
			if text, ok := item.(string); ok {
				blocks("user", text)
				continue
			}
			message, _ := item.(map[string]any)
			role := jsonString(message, "role")
			if role == "" {
				role = "user"
			}
			switch jsonString(message, "type") {
			case "function_call", "function_call_output", "reasoning":
				add("tool", jsonString(message, "type"), conversationJSONText(message))
			default:
				if content, ok := message["content"]; ok {
					blocks(role, content)
				} else {
					blocks(role, message["text"])
				}
			}
		}
	case map[string]any:
		blocks(firstNonEmpty(jsonString(input, "role"), "user"), input["content"])
	}
	if contents, ok := root["contents"].([]any); ok {
		for _, item := range contents {
			message, _ := item.(map[string]any)
			blocks(firstNonEmpty(jsonString(message, "role"), "user"), message["parts"])
		}
	}
	if tools := root["tools"]; tools != nil {
		add("system", "tool_definitions", conversationJSONText(tools))
	}
	if len(details.Messages) == 0 && len(details.Context) == 0 {
		return details, "", 0, false, ErrNoPromptText
	}
	kind := conversationKindDialogue
	if len(details.Messages) == 0 {
		kind = conversationKindUnknown
	}
	answer := strings.TrimSpace(response)
	if monitor && (answer == "<block>no</block>" || strings.HasPrefix(answer, "<block>yes</block>")) {
		kind = conversationKindAuxiliary
		details.ClassificationReason = "security_monitor_signature"
	}
	var total int
	for _, message := range details.Messages {
		if message.Position == details.CurrentMessagePosition {
			details.UserPreview, _, _ = trimConversationText(message.Content, 120)
		}
	}
	if kind == conversationKindDialogue {
		for index, message := range details.Messages {
			if message.Position == details.CurrentMessagePosition && index > 0 {
				details.ParentHistoryKey = conversationHistoryKey(request, details.Messages[:index])
			}
		}
		if strings.TrimSpace(response) != "" {
			next := append(append([]ConversationMessage(nil), details.Messages...), ConversationMessage{Role: "assistant", Content: response})
			details.HistoryKey = conversationHistoryKey(request, next)
		}
	}
	for _, message := range details.Messages {
		total += utf8.RuneCountInString(message.Content)
	}
	var truncated bool
	details.Messages, details.OmittedMessages, truncated = boundConversationMessages(details.Messages, limit, details.CurrentMessagePosition)
	details.Context, details.OmittedContext, details.ContextTruncated = boundConversationMessages(details.Context, limit, -1)
	return details, kind, total, truncated, nil
}

func conversationHistoryKey(request Request, messages []ConversationMessage) string {
	items := make([][2]string, 0, len(messages))
	for _, message := range messages {
		items = append(items, [2]string{message.Role, strings.TrimSpace(message.Content)})
	}
	raw, _ := json.Marshal([]any{request.UserID, request.APIKeyID, request.Protocol, items})
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:])
}

func conversationJSONText(value any) string { raw, _ := json.Marshal(value); return string(raw) }

func boundConversationMessages(messages []ConversationMessage, limit, priority int) ([]ConversationMessage, int, bool) {
	order := make([]int, 0, len(messages))
	for index, message := range messages {
		if message.Position == priority {
			order = append(order, index)
		}
	}
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index].Position != priority {
			order = append(order, index)
		}
	}
	kept := make([]ConversationMessage, 0)
	truncated := false
	for _, index := range order {
		if limit <= 0 || len(kept) >= conversationMessageLimit {
			truncated = true
			continue
		}
		message := messages[index]
		message.Content, _, message.Truncated = trimConversationText(message.Content, limit)
		truncated = truncated || message.Truncated
		limit -= utf8.RuneCountInString(message.Content)
		kept = append(kept, message)
	}
	sort.Slice(kept, func(i, j int) bool { return kept[i].Position < kept[j].Position })
	return kept, len(messages) - len(kept), truncated
}

func conversationMessagesTranscript(messages []ConversationMessage) string {
	parts := make([]string, 0, len(messages))
	for _, message := range messages {
		parts = append(parts, "["+message.Role+"]\n"+message.Content)
	}
	return strings.Join(parts, "\n\n")
}

func captureConversationResponseContext(raw []byte, limit int) ([]ConversationMessage, bool) {
	var roots []map[string]any
	var root map[string]any
	if json.Unmarshal(raw, &root) == nil {
		roots = append(roots, root)
	} else {
		for _, line := range strings.Split(string(raw), "\n") {
			if !strings.HasPrefix(strings.TrimSpace(line), "data:") {
				continue
			}
			var frame map[string]any
			if json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "data:"))), &frame) == nil {
				roots = append(roots, frame)
			}
		}
	}
	context := make([]ConversationMessage, 0)
	add := func(kind string, value any) {
		context = append(context, ConversationMessage{Role: "assistant", Kind: kind, Content: conversationJSONText(value), Position: len(context)})
	}
	for _, root := range roots {
		if response, ok := root["response"].(map[string]any); ok {
			root = response
		}
		if choices, ok := root["choices"].([]any); ok {
			for _, item := range choices {
				choice, _ := item.(map[string]any)
				for _, key := range []string{"message", "delta"} {
					message, _ := choice[key].(map[string]any)
					if calls := message["tool_calls"]; calls != nil {
						add("tool_use", calls)
					}
					if call := message["function_call"]; call != nil {
						add("tool_use", call)
					}
				}
			}
		}
		for _, key := range []string{"content", "output"} {
			items, _ := root[key].([]any)
			for _, item := range items {
				block, _ := item.(map[string]any)
				switch jsonString(block, "type") {
				case "tool_use", "function_call", "reasoning", "thinking":
					add(jsonString(block, "type"), block)
				}
			}
		}
		if block, ok := root["content_block"].(map[string]any); ok && jsonString(block, "type") == "tool_use" {
			add("tool_use", block)
		}
		if delta, ok := root["delta"].(map[string]any); ok && jsonString(delta, "type") == "input_json_delta" {
			add("tool_use_delta", delta)
		}
		if jsonString(root, "type") == "response.function_call_arguments.delta" {
			add("tool_use_delta", root)
		}
		if candidates, ok := root["candidates"].([]any); ok {
			for _, item := range candidates {
				candidate, _ := item.(map[string]any)
				content, _ := candidate["content"].(map[string]any)
				parts, _ := content["parts"].([]any)
				for _, item := range parts {
					part, _ := item.(map[string]any)
					if call := part["functionCall"]; call != nil {
						add("tool_use", call)
					}
				}
			}
		}
	}
	kept, _, truncated := boundConversationMessages(context, limit, -1)
	return kept, truncated
}
