package securityaudit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return ""
	}
	if strings.Contains(trimmed, "data:") || strings.Contains(trimmed, "event:") {
		return normalizeSSEResponse(protocol, []byte(trimmed))
	}
	var root map[string]any
	if json.Unmarshal(body, &root) != nil {
		return ""
	}
	return strings.TrimSpace(strings.Join(responseTexts(protocol, root), "\n"))
}

func normalizeSSEResponse(protocol string, body []byte) string {
	frames := strings.FieldsFunc(string(body), func(r rune) bool { return r == '\n' || r == '\r' })
	deltas := make([]string, 0, len(frames))
	final := ""
	for _, line := range frames {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var root map[string]any
		if json.Unmarshal([]byte(payload), &root) != nil {
			continue
		}
		texts := responseTexts(protocol, root)
		if isIncrementalResponse(protocol, root) {
			deltas = append(deltas, texts...)
		} else if len(texts) > 0 {
			final = strings.Join(texts, "\n")
		}
	}
	if len(deltas) > 0 {
		return strings.TrimSpace(strings.Join(deltas, ""))
	}
	return strings.TrimSpace(final)
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
	result := make([]string, 0, 4)
	appendText := func(value any) {
		for _, text := range contentTexts(value) {
			if text = strings.TrimSpace(text); text != "" {
				result = append(result, text)
			}
		}
	}
	if value, ok := root["output_text"].(string); ok && strings.TrimSpace(value) != "" {
		return []string{value}
	}
	if delta, ok := root["delta"].(string); ok && strings.Contains(strings.ToLower(jsonString(root, "type")), "text") {
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
	value = strings.ReplaceAll(strings.TrimSpace(value), "\x00", "")
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
