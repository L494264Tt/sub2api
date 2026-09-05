package securityaudit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"
)

func conversationIdentity(request Request, responseBody []byte) (externalID, previousResponseID, responseID string) {
	var root map[string]any
	_ = json.Unmarshal(request.Body, &root)
	externalID = firstNonEmpty(
		request.ConversationID,
		jsonString(root, "conversation_id"), jsonString(root, "session_id"), jsonString(root, "thread_id"),
		jsonNestedString(root, "metadata", "conversation_id"),
	)
	previousResponseID = jsonString(root, "previous_response_id")
	responseID = responseObjectID(responseBody)
	return
}

func conversationKey(request Request, externalID, previousResponseID string) string {
	identity := externalID
	if identity == "" {
		identity = previousResponseID
	}
	if identity == "" {
		identity = request.RequestID
	}
	raw := strings.Join([]string{
		int64String(request.UserID), int64String(request.APIKeyID), strings.TrimSpace(request.Protocol), identity,
	}, "\x00")
	digest := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(digest[:])
}

func normalizeAssistantResponse(protocol string, body []byte) string {
	return normalizeCapturedAssistantResponse(protocol, "", body, false)
}

func normalizeCapturedAssistantResponse(protocol, contentType string, body []byte, truncated bool) string {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return ""
	}
	var root map[string]any
	if json.Unmarshal(body, &root) == nil {
		return strings.Join(responseTexts(protocol, root), "\n")
	}
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		if truncated {
			root, _ := decodeConversationJSONPrefix(body).(map[string]any)
			return strings.Join(responseTexts(protocol, root), "\n")
		}
		return ""
	}
	if strings.Contains(strings.ToLower(contentType), "text/event-stream") ||
		strings.HasPrefix(trimmed, "data:") || strings.HasPrefix(trimmed, "event:") || strings.HasPrefix(trimmed, ":") {
		return normalizeSSEResponse(protocol, body, truncated)
	}
	return ""
}

func conversationTranscript(request Request) (string, error) {
	var document any
	if err := json.Unmarshal(request.Body, &document); err != nil {
		return "", errors.New("conversation request JSON is invalid")
	}
	segments := extractProtocolSegments(request.Protocol, document)
	parts := make([]string, 0, len(segments))
	for _, segment := range segments {
		if strings.TrimSpace(segment.text) == "" {
			continue
		}
		role := segment.role
		if role == "" {
			role = "user"
		}
		parts = append(parts, "["+role+"]\n"+segment.text)
	}
	if len(parts) == 0 {
		return "", ErrNoPromptText
	}
	return strings.Join(parts, "\n\n"), nil
}

func normalizeSSEResponse(protocol string, body []byte, truncated bool) string {
	frames := strings.FieldsFunc(string(body), func(r rune) bool { return r == '\n' || r == '\r' })
	deltas := make([]string, 0, len(frames))
	final := ""
	for index, line := range frames {
		line = strings.TrimLeft(line, " \t")
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimLeft(strings.TrimPrefix(line, "data:"), " \t")
		if strings.TrimSpace(payload) == "" || strings.TrimSpace(payload) == "[DONE]" {
			continue
		}
		var root map[string]any
		if json.Unmarshal([]byte(payload), &root) != nil {
			if !truncated || index != len(frames)-1 {
				continue
			}
			root, _ = decodeConversationJSONPrefix([]byte(payload)).(map[string]any)
		}
		texts := responseTexts(protocol, root)
		if isIncrementalResponse(protocol, root) {
			deltas = append(deltas, texts...)
		} else if len(texts) > 0 {
			final = strings.Join(texts, "\n")
		}
	}
	if len(deltas) > 0 {
		return strings.Join(deltas, "")
	}
	return final
}

func isIncrementalResponse(protocol string, root map[string]any) bool {
	if strings.Contains(strings.ToLower(jsonString(root, "type")), "delta") {
		return true
	}
	if choices, ok := root["choices"].([]any); ok {
		for _, item := range choices {
			choice, _ := item.(map[string]any)
			if _, ok := choice["delta"].(map[string]any); ok {
				return true
			}
		}
	}
	return strings.Contains(strings.ToLower(protocol), "gemini")
}

func responseTexts(protocol string, root map[string]any) []string {
	if root == nil {
		return nil
	}
	if strings.Contains(jsonString(root, "type"), "reasoning") {
		return nil
	}
	result := make([]string, 0, 4)
	appendText := func(value any) {
		result = append(result, responseContentTexts(value)...)
	}
	if value, ok := root["output_text"].(string); ok && strings.TrimSpace(value) != "" {
		return []string{value}
	}
	if delta, ok := root["delta"].(string); ok && jsonString(root, "type") == "response.output_text.delta" {
		result = append(result, delta)
	}
	if delta, ok := root["delta"].(map[string]any); ok {
		appendText(delta["text"])
	}
	if content, exists := root["content"]; exists {
		appendText(content)
	}
	if choices, ok := root["choices"].([]any); ok {
		for _, item := range choices {
			choice, _ := item.(map[string]any)
			if message, ok := choice["message"].(map[string]any); ok {
				appendText(message["content"])
			}
			if delta, ok := choice["delta"].(map[string]any); ok {
				appendText(delta["content"])
			}
		}
	}
	if output, ok := root["output"].([]any); ok {
		for _, item := range output {
			entry, _ := item.(map[string]any)
			if jsonString(entry, "type") == "reasoning" {
				continue
			}
			appendText(entry["content"])
		}
	}
	if candidates, ok := root["candidates"].([]any); ok {
		for _, item := range candidates {
			candidate, _ := item.(map[string]any)
			if content, ok := candidate["content"].(map[string]any); ok {
				appendText(content["parts"])
			}
		}
	}
	if response, ok := root["response"].(map[string]any); ok {
		result = append(result, responseTexts(protocol, response)...)
	}
	_ = protocol
	return result
}

// Unlike prompt normalization, response chunks must retain whitespace: a space
// or newline can be an entire streaming delta.
func responseContentTexts(value any) []string {
	switch value := value.(type) {
	case string:
		return []string{value}
	case []any:
		var result []string
		for _, item := range value {
			result = append(result, responseContentTexts(item)...)
		}
		return result
	case map[string]any:
		if thought, _ := value["thought"].(bool); thought {
			return nil
		}
		switch jsonString(value, "type") {
		case "", "text", "input_text", "output_text":
			if text, ok := value["text"].(string); ok {
				return []string{text}
			}
		}
	}
	return nil
}

func responseObjectID(body []byte) string {
	var root map[string]any
	if json.Unmarshal(body, &root) != nil {
		for _, line := range strings.FieldsFunc(string(body), func(r rune) bool { return r == '\n' || r == '\r' }) {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			if json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &root) != nil {
				continue
			}
			if response, ok := root["response"].(map[string]any); ok {
				if id := jsonString(response, "id"); id != "" {
					return id
				}
			}
			if id := jsonString(root, "id"); id != "" {
				return id
			}
		}
		return ""
	}
	if id := jsonString(root, "id"); id != "" {
		return id
	}
	if response, ok := root["response"].(map[string]any); ok {
		return jsonString(response, "id")
	}
	return ""
}

func trimConversationText(value string, limit int) (string, int, bool) {
	value = strings.ReplaceAll(value, "\x00", "")
	count := utf8.RuneCountInString(value)
	if limit <= 0 || count <= limit {
		return value, count, false
	}
	runes := []rune(value)
	return string(runes[:limit]), count, true
}

func jsonString(root map[string]any, key string) string {
	if root == nil {
		return ""
	}
	value, _ := root[key].(string)
	return strings.TrimSpace(value)
}

func jsonNestedString(root map[string]any, objectKey, valueKey string) string {
	object, _ := root[objectKey].(map[string]any)
	return jsonString(object, valueKey)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func int64String(value int64) string {
	if value == 0 {
		return "0"
	}
	const digits = "0123456789"
	negative := value < 0
	if negative {
		value = -value
	}
	var buffer [20]byte
	index := len(buffer)
	for value > 0 {
		index--
		buffer[index] = digits[value%10]
		value /= 10
	}
	if negative {
		index--
		buffer[index] = '-'
	}
	return string(buffer[index:])
}
