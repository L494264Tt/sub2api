package usagearchive

import (
	"testing"
)

func TestExtractMessagesSeparatesPromptRolesForOpenAIChat(t *testing.T) {
	body := []byte(`{"messages":[{"role":"system","content":"policy"},{"role":"developer","content":"developer rules"},{"role":"user","content":"question"},{"role":"assistant","content":"old answer"},{"role":"tool","content":"tool result"}]}`)
	got, err := ExtractMessages("openai_chat_completions", body)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 {
		t.Fatalf("got %d messages, want 5", len(got))
	}
	if got[0].Role != RoleSystem || got[1].Role != RoleDeveloper || got[2].Role != RoleUser || got[3].Role != RoleAssistant || got[4].Role != RoleTool {
		t.Fatalf("unexpected roles: %#v", got)
	}
	if got[2].Content != "question" {
		t.Fatalf("user content = %q", got[2].Content)
	}
}

func TestExtractMessagesSupportsAnthropicSystemAndGeminiModelRole(t *testing.T) {
	anthropic, err := ExtractMessages("anthropic_messages", []byte(`{"system":"policy","messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`))
	if err != nil || len(anthropic) != 2 || anthropic[0].Role != RoleSystem || anthropic[1].Role != RoleUser {
		t.Fatalf("anthropic extraction = %#v, err=%v", anthropic, err)
	}
	gemini, err := ExtractMessages("gemini", []byte(`{"systemInstruction":{"parts":[{"text":"policy"}]},"contents":[{"role":"user","parts":[{"text":"hello"}]},{"role":"model","parts":[{"text":"answer"}]}]}`))
	if err != nil || len(gemini) != 3 || gemini[2].Role != RoleAssistant {
		t.Fatalf("gemini extraction = %#v, err=%v", gemini, err)
	}
}
