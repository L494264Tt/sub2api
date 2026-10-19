package usagearchive

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

func ExtractMessages(protocol string, body []byte) ([]ArchiveMessage, error) {
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, fmt.Errorf("usage request JSON is invalid: %w", err)
	}
	var messages []ArchiveMessage
	add := func(role MessageRole, source MessageSource, content string) {
		content = strings.TrimSpace(content)
		if content == "" {
			return
		}
		if ValidateMessageRole(role) != nil {
			role = RoleUnknown
		}
		messages = append(messages, newArchiveMessage(role, source, len(messages), content))
	}
	addBlocks := func(role MessageRole, source MessageSource, value any) {
		for _, text := range contentTexts(value) {
			add(role, source, text)
		}
	}

	protocol = strings.ToLower(strings.TrimSpace(protocol))
	if strings.Contains(protocol, "gemini") {
		if system, ok := root["systemInstruction"]; ok {
			addBlocks(RoleSystem, SourceClientRequest, system)
		}
		if system, ok := root["system_instruction"]; ok {
			addBlocks(RoleSystem, SourceClientRequest, system)
		}
		addGeminiContents(root["contents"], add)
		return messages, nil
	}

	if system, ok := root["system"]; ok {
		addBlocks(RoleSystem, SourceClientRequest, system)
	}
	if instructions, ok := root["instructions"]; ok {
		addBlocks(RoleDeveloper, SourceClientRequest, instructions)
	}
	if input, ok := root["input"]; ok {
		addOpenAIInput(input, add)
	}
	if rawMessages, ok := root["messages"].([]any); ok {
		for _, raw := range rawMessages {
			entry, _ := raw.(map[string]any)
			role := normalizeRole(stringValue(entry["role"]))
			addBlocks(role, SourceClientRequest, entry["content"])
			if toolCalls, ok := entry["tool_calls"]; ok {
				addBlocks(RoleTool, SourceTool, toolCalls)
			}
		}
	}
	return messages, nil
}

func newArchiveMessage(role MessageRole, source MessageSource, ordinal int, content string) ArchiveMessage {
	digest := sha256.Sum256([]byte(content))
	return ArchiveMessage{
		Role:         role,
		Source:       source,
		Ordinal:      ordinal,
		Content:      content,
		ContentHash:  hex.EncodeToString(digest[:]),
		ContentBytes: len([]byte(content)),
		ContentRunes: utf8.RuneCountInString(content),
	}
}

func normalizeRole(value string) MessageRole {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "system":
		return RoleSystem
	case "developer":
		return RoleDeveloper
	case "user":
		return RoleUser
	case "assistant", "model":
		return RoleAssistant
	case "tool", "function":
		return RoleTool
	default:
		return RoleUnknown
	}
}

func addOpenAIInput(value any, add func(MessageRole, MessageSource, string)) {
	if text, ok := value.(string); ok {
		add(RoleUser, SourceClientRequest, text)
		return
	}
	items, _ := value.([]any)
	for _, raw := range items {
		entry, _ := raw.(map[string]any)
		role := normalizeRole(stringValue(entry["role"]))
		if role == RoleUnknown {
			role = RoleUser
		}
		for _, text := range contentTexts(entry["content"]) {
			add(role, SourceClientRequest, text)
		}
	}
}

func addGeminiContents(value any, add func(MessageRole, MessageSource, string)) {
	items, _ := value.([]any)
	for _, raw := range items {
		entry, _ := raw.(map[string]any)
		role := normalizeRole(stringValue(entry["role"]))
		if role == RoleUnknown {
			role = RoleUser
		}
		addBlocks := func(value any) {
			for _, text := range contentTexts(value) {
				add(role, SourceClientRequest, text)
			}
		}
		addBlocks(entry["parts"])
	}
}

func contentTexts(value any) []string {
	switch typed := value.(type) {
	case string:
		return []string{typed}
	case []any:
		var result []string
		for _, item := range typed {
			result = append(result, contentTexts(item)...)
		}
		return result
	case map[string]any:
		if text, ok := typed["text"].(string); ok {
			return []string{text}
		}
		if parts, ok := typed["parts"]; ok {
			return contentTexts(parts)
		}
		if function, ok := typed["function"].(map[string]any); ok {
			return contentTexts(function)
		}
		if name, ok := typed["name"].(string); ok {
			if args, ok := typed["args"]; ok {
				encoded, _ := json.Marshal(args)
				return []string{name + " " + string(encoded)}
			}
			return []string{name}
		}
	}
	return nil
}

func stringValue(value any) string {
	result, _ := value.(string)
	return result
}
