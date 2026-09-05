package securityaudit

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"unicode/utf8"
)

const conversationJSONMaxDepth = 128

// Decode only a known capture-limit prefix. Normal responses still require
// valid JSON. Token decoding preserves completed fields and nesting without
// searching raw JSON for text-looking substrings.
func decodeConversationJSONPrefix(raw []byte) any {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	value, _ := readConversationJSONPrefix(decoder, raw, 0)
	return value
}

func readConversationJSONPrefix(decoder *json.Decoder, raw []byte, depth int) (any, error) {
	if depth >= conversationJSONMaxDepth {
		return nil, io.ErrUnexpectedEOF
	}
	offset := decoder.InputOffset()
	token, err := decoder.Token()
	if err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return conversationJSONStringPrefix(raw[offset:]), err
		}
		return nil, err
	}
	switch token {
	case json.Delim('{'):
		object := make(map[string]any)
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return object, err
			}
			name, ok := key.(string)
			if !ok {
				return object, io.ErrUnexpectedEOF
			}
			value, err := readConversationJSONPrefix(decoder, raw, depth+1)
			if value != nil {
				object[name] = value
			}
			if err != nil {
				return object, err
			}
		}
		_, err := decoder.Token()
		return object, err
	case json.Delim('['):
		array := make([]any, 0)
		for decoder.More() {
			value, err := readConversationJSONPrefix(decoder, raw, depth+1)
			if value != nil {
				array = append(array, value)
			}
			if err != nil {
				return array, err
			}
		}
		_, err := decoder.Token()
		return array, err
	default:
		return token, nil
	}
}

func conversationJSONStringPrefix(raw []byte) any {
	prefix := strings.TrimLeft(string(raw), " \t\r\n,:")
	if !strings.HasPrefix(prefix, `"`) {
		return nil
	}
	// At most an incomplete surrogate pair (12 bytes) or UTF-8 rune may need
	// removing from the end. Never repair invalid data elsewhere in the string.
	const maxIncompleteEscapeBytes = 12
	for removed := 0; removed <= maxIncompleteEscapeBytes && len(prefix) > 0; removed++ {
		if utf8.ValidString(prefix) {
			var value string
			if json.Unmarshal([]byte(prefix+`"`), &value) == nil && !strings.HasSuffix(value, "\uFFFD") {
				return value
			}
		}
		prefix = prefix[:len(prefix)-1]
	}
	return nil
}
