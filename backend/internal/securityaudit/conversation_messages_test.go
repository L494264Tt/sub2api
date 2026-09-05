package securityaudit

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConversationMessageBudgetProtectsUserInput(t *testing.T) {
	body, _ := json.Marshal(map[string]any{"system": strings.Repeat("policy ", 20000), "messages": []any{
		map[string]any{"role": "user", "content": "old question"}, map[string]any{"role": "assistant", "content": strings.Repeat("old answer ", 100)}, map[string]any{"role": "user", "content": "latest question"},
	}})
	details, kind, _, truncated, err := captureConversationMessages(Request{Protocol: "anthropic_messages", Body: body}, "answer", 128)
	require.NoError(t, err)
	require.Equal(t, "conversation", kind)
	require.True(t, details.ContextTruncated)
	require.True(t, truncated)
	last := details.Messages[len(details.Messages)-1]
	require.Equal(t, "latest question", last.Content)
	require.False(t, last.Truncated)
	require.Equal(t, last.Position, details.CurrentMessagePosition)
	require.NotContains(t, conversationMessagesTranscript(details.Messages), "policy")
}

func TestConversationToolContextDoesNotBecomeUserDialogue(t *testing.T) {
	tests := []struct{ protocol, body string }{
		{"anthropic_messages", `{"system":"policy","messages":[{"role":"user","content":[{"type":"text","text":"real question"},{"type":"tool_result","tool_use_id":"call_1","content":"tool secret"}]},{"role":"assistant","content":[{"type":"tool_use","name":"run","input":{"cmd":"ls"}}]}]}`},
		{"openai_chat", `{"messages":[{"role":"system","content":"policy"},{"role":"user","content":"real question"},{"role":"assistant","tool_calls":[{"function":{"name":"run","arguments":"{}"}}]},{"role":"tool","content":"tool secret"}]}`},
		{"openai_responses", `{"instructions":"policy","input":[{"role":"user","content":"real question"},{"type":"function_call","name":"run","arguments":"{}"},{"type":"function_call_output","output":"tool secret"}]}`},
		{"gemini", `{"systemInstruction":{"parts":[{"text":"policy"}]},"contents":[{"role":"user","parts":[{"text":"real question"}]},{"role":"model","parts":[{"functionCall":{"name":"run","args":{}}}]},{"role":"user","parts":[{"functionResponse":{"response":"tool secret"}}]}]}`},
	}
	for _, test := range tests {
		t.Run(test.protocol, func(t *testing.T) {
			details, _, _, _, err := captureConversationMessages(Request{Protocol: test.protocol, Body: []byte(test.body)}, "answer", 1024)
			require.NoError(t, err)
			require.Len(t, details.Messages, 1)
			require.Equal(t, "real question", details.Messages[0].Content)
			require.Contains(t, conversationMessagesTranscript(details.Context), "tool secret")
		})
	}
}

func TestConversationAuxiliaryClassificationRequiresBothSignals(t *testing.T) {
	for _, test := range []struct{ system, response, want string }{
		{securityMonitorSignature, "<block>no</block>", "auxiliary"},
		{"Explain the following quote: " + securityMonitorSignature, "<block>no</block>", "conversation"},
		{securityMonitorSignature, "normal answer", "conversation"},
	} {
		body, _ := json.Marshal(map[string]any{"system": test.system, "messages": []any{map[string]any{"role": "user", "content": "input"}}})
		_, kind, _, _, err := captureConversationMessages(Request{Body: body}, test.response, 128)
		require.NoError(t, err)
		require.Equal(t, test.want, kind)
	}
}

func TestConversationHistoryKeysLinkFullHistoryAndIsolateUsers(t *testing.T) {
	first := Request{UserID: 1, APIKeyID: 2, Protocol: "openai_chat", Body: []byte(`{"messages":[{"role":"user","content":"question one"}]}`)}
	previous, _, _, _, err := captureConversationMessages(first, "answer one", 128)
	require.NoError(t, err)
	next := first
	next.Body = []byte(`{"messages":[{"role":"system","content":"new policy"},{"role":"user","content":"question one"},{"role":"assistant","content":"answer one"},{"role":"user","content":"question two"}]}`)
	current, _, _, _, err := captureConversationMessages(next, "answer two", 128)
	require.NoError(t, err)
	require.NotEmpty(t, current.ParentHistoryKey)
	require.Equal(t, previous.HistoryKey, current.ParentHistoryKey)
	next.UserID = 3
	other, _, _, _, err := captureConversationMessages(next, "answer two", 128)
	require.NoError(t, err)
	require.NotEqual(t, previous.HistoryKey, other.ParentHistoryKey)
}

func TestConversationResponseToolCallsAreRetained(t *testing.T) {
	for _, body := range []string{
		`{"choices":[{"message":{"tool_calls":[{"function":{"name":"run","arguments":"{}"}}]}}]}`,
		`{"content":[{"type":"tool_use","name":"run","input":{}}]}`,
		`{"output":[{"type":"function_call","name":"run","arguments":"{}"}]}`,
		`{"candidates":[{"content":{"parts":[{"functionCall":{"name":"run"}}]}}]}`,
	} {
		context, truncated := captureConversationResponseContext([]byte(body), 128)
		require.NotEmpty(t, context)
		require.False(t, truncated)
		require.Contains(t, context[0].Content, "run")
	}
}

func TestInjectedClientUserContextGetsItsOwnBudget(t *testing.T) {
	injected := "# AGENTS.md instructions for /repo\n<INSTRUCTIONS>" + strings.Repeat("policy ", 1000) + "</INSTRUCTIONS>\n<environment_context>workspace</environment_context>\nreal question"
	body, _ := json.Marshal(map[string]any{"messages": []any{map[string]any{"role": "user", "content": injected}, map[string]any{"role": "user", "content": "<skill><name>helper</name>instructions</skill>"}}})
	details, _, _, truncated, err := captureConversationMessages(Request{Body: body}, "answer", 128)
	require.NoError(t, err)
	require.Len(t, details.Messages, 1)
	require.Equal(t, "real question", details.Messages[0].Content)
	require.False(t, truncated)
	require.True(t, details.ContextTruncated)
	for _, text := range []string{"Explain <skill>helper</skill>", "<skill>unfinished", "# AGENTS.md instructions for /repo\n<INSTRUCTIONS>unfinished"} {
		remaining, context := splitConversationClientContext(text)
		require.Equal(t, text, remaining)
		require.Empty(t, context)
	}
}

func TestHeartbeatAndSuggestionChecksAreAuxiliary(t *testing.T) {
	for _, test := range []struct{ prompt, answer, reason string }{
		{"[Sat 2026-09-05 07:21 UTC] [OpenClaw heartbeat poll]", "HEARTBEAT_OK", "heartbeat_poll"},
		{suggestionSafetySignature + "\npolicy", `{"exclude":[]}`, "suggestion_safety_check"},
	} {
		body, _ := json.Marshal(map[string]any{"messages": []any{map[string]any{"role": "user", "content": test.prompt}}})
		details, kind, _, _, err := captureConversationMessages(Request{Body: body}, test.answer, 128)
		require.NoError(t, err)
		require.Equal(t, conversationKindAuxiliary, kind)
		require.Equal(t, test.reason, details.ClassificationReason)
	}
}

func TestActionApprovalRequestsAreAuxiliary(t *testing.T) {
	body, _ := json.Marshal(map[string]any{"system": actionReviewSignature, "messages": []any{map[string]any{"role": "user", "content": "planned action"}}})
	details, kind, _, _, err := captureConversationMessages(Request{Body: body}, `{"outcome":"allow"}`, 128)
	require.NoError(t, err)
	require.Equal(t, conversationKindAuxiliary, kind)
	require.Equal(t, "action_approval_review", details.ClassificationReason)
}

func TestReasoningSummariesStayOutsideVisibleResponse(t *testing.T) {
	body := []byte("data: {\"type\":\"response.reasoning_summary_text.delta\",\"delta\":\"internal heading\"}\n\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"actual reply\"}\n\n")
	require.Equal(t, "actual reply", normalizeAssistantResponse("openai_responses", body))
	context, _ := captureConversationResponseContext(body, 1024)
	require.Len(t, context, 1)
	require.Equal(t, "reasoning", context[0].Kind)
	gemini := []byte(`{"candidates":[{"content":{"parts":[{"thought":true,"text":"internal"},{"text":"actual"}]}}]}`)
	require.Equal(t, "actual", normalizeAssistantResponse("gemini", gemini))
}
