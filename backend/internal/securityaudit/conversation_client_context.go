package securityaudit

import "strings"

var conversationClientContextTags = []string{"environment_context", "skills_instructions", "skill", "permissions", "collaboration_mode"}

// Client-injected envelopes are often sent with role=user. Keep them in the
// context budget, preserving any actual user text following a complete block.
func splitConversationClientContext(text string) (string, []string) {
	remaining := strings.TrimSpace(text)
	var context []string
	if strings.HasPrefix(remaining, "# AGENTS.md instructions for ") && strings.Contains(remaining, "<INSTRUCTIONS>") {
		end := strings.Index(remaining, "</INSTRUCTIONS>")
		if end < 0 {
			return text, nil
		}
		end += len("</INSTRUCTIONS>")
		context = append(context, remaining[:end])
		remaining = strings.TrimSpace(remaining[end:])
	}
	for {
		matched := false
		for _, tag := range conversationClientContextTags {
			if !strings.HasPrefix(remaining, "<"+tag+">") {
				continue
			}
			end := strings.Index(remaining, "</"+tag+">")
			if end < 0 {
				continue
			}
			end += len(tag) + 3
			context = append(context, remaining[:end])
			remaining = strings.TrimSpace(remaining[end:])
			matched = true
			break
		}
		if !matched {
			break
		}
	}
	if len(context) == 0 {
		return text, nil
	}
	return remaining, context
}
