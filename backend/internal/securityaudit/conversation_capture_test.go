package securityaudit

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNormalizeAssistantResponse(t *testing.T) {
	tests := []struct {
		name     string
		protocol string
		body     string
		want     string
	}{
		{name: "JSON containing SSE vocabulary", protocol: "openai_chat", body: `{"choices":[{"message":{"content":"Use data: and event: fields"}}]}`, want: "Use data: and event: fields"},
		{name: "chat stream whitespace", protocol: "openai_chat", body: "data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\" world\\n\"}}]}\n\n", want: "Hello world\n"},
		{name: "anthropic stream whitespace", protocol: "anthropic_messages", body: "data: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"a \"}}\n\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\" b\"}}\n\n", want: "a  b"},
		{name: "gemini stream whitespace", protocol: "gemini", body: "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"a \"}]}}]}\n\ndata: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\" b\"}]}}]}\n\n", want: "a  b"},
		{name: "chat completion", protocol: "openai_chat_completions", body: `{"choices":[{"message":{"content":"hello"}}]}`, want: "hello"},
		{name: "responses", protocol: "openai_responses", body: `{"output_text":"answer","output":[{"content":[{"type":"output_text","text":"answer"}]}]}`, want: "answer"},
		{name: "anthropic", protocol: "anthropic_messages", body: `{"content":[{"type":"text","text":"claude"}]}`, want: "claude"},
		{name: "gemini", protocol: "gemini_generate_content", body: `{"candidates":[{"content":{"parts":[{"text":"gemini"}]}}]}`, want: "gemini"},
		{name: "stream deltas win over final frame", protocol: "openai_responses", body: strings.Join([]string{
			`event: response.output_text.delta`, `data: {"type":"response.output_text.delta","delta":"hel"}`, "",
			`event: response.output_text.delta`, `data: {"type":"response.output_text.delta","delta":"lo"}`, "",
			`event: response.completed`, `data: {"type":"response.completed","response":{"output_text":"hello"}}`, "",
		}, "\n"), want: "hello"},
		{name: "chat completion stream", protocol: "openai_chat_completions", body: strings.Join([]string{
			`data: {"choices":[{"delta":{"content":"hel"}}]}`, "",
			`data: {"choices":[{"delta":{"content":"lo"}}]}`, "", `data: [DONE]`, "",
		}, "\n"), want: "hello"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, normalizeAssistantResponse(test.protocol, []byte(test.body)))
		})
	}
}

func TestConversationTranscriptPreservesOrderAndRoles(t *testing.T) {
	for _, protocol := range []string{"openai_chat", "anthropic_messages"} {
		request := Request{Protocol: protocol, Body: []byte(`{"messages":[{"role":"user","content":"first question"},{"role":"assistant","content":"first answer"},{"role":"user","content":"second question"}]}`)}
		got, err := conversationTranscript(request)
		require.NoError(t, err)
		require.Equal(t, "[user]\nfirst question\n\n[assistant]\nfirst answer\n\n[user]\nsecond question", got)
	}
	for _, request := range []Request{
		{Protocol: "openai_responses", Body: []byte(`{"instructions":"policy","input":[{"role":"user","content":"question"},{"role":"assistant","content":"answer"}]}`)},
		{Protocol: "gemini", Body: []byte(`{"systemInstruction":{"parts":[{"text":"policy"}]},"contents":[{"role":"user","parts":[{"text":"question"}]},{"role":"model","parts":[{"text":"answer"}]}]}`)},
	} {
		got, err := conversationTranscript(request)
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(got, "[system]\npolicy\n\n[user]\nquestion"))
		require.True(t, strings.HasSuffix(got, "\nanswer"))
	}
}

func TestTruncatedConversationResponsesRetainRecoverableText(t *testing.T) {
	tests := []struct{ name, protocol, body, want string }{
		{"chat JSON string", "openai_chat", `{"choices":[{"message":{"content":"Hello world`, "Hello world"},
		{"responses JSON", "openai_responses", `{"output":[{"content":[{"type":"output_text","text":"hello `, "hello "},
		{"escaped text", "anthropic_messages", `{"content":[{"type":"text","text":"line\nnext\u4`, "line\nnext"},
		{"surrogate pair", "openai_responses", `{"output_text":"hello\uD83D\uDE`, "hello"},
		{"later metadata truncated", "openai_chat", `{"choices":[{"message":{"content":"kept"}}],"usage":{"total_tokens":`, "kept"},
		{"complete delta and partial delta", "openai_chat", "data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\" world ", "Hello world "},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, normalizeCapturedAssistantResponse(test.protocol, "", []byte(test.body), true))
		})
	}
	require.Empty(t, normalizeCapturedAssistantResponse("openai_chat", "application/json", []byte(tests[0].body), false))
	unicodeBody := []byte(`{"output_text":"hello ` + "世")
	require.Equal(t, "hello ", normalizeCapturedAssistantResponse("openai_responses", "application/json", unicodeBody[:len(unicodeBody)-1], true))
}

func TestConversationIdentityUsesExplicitHeaderAndFindsStreamResponseID(t *testing.T) {
	request := Request{
		RequestID: "request-1", UserID: 7, APIKeyID: 9, Protocol: "openai_responses",
		ConversationID: "conversation-from-header",
		Body:           []byte(`{"conversation_id":"body-value","previous_response_id":"resp_previous"}`),
	}
	body := []byte("event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_current\"}}\n\n")
	external, previous, response := conversationIdentity(request, body)
	require.Equal(t, "conversation-from-header", external)
	require.Equal(t, "resp_previous", previous)
	require.Equal(t, "resp_current", response)
	require.Len(t, conversationKey(request, external, previous), 64)
}

func TestConversationConfigDefaultsOffAndNormalizesLegacyConfig(t *testing.T) {
	cfg, err := ParseStorageConfig(`{"enabled":false,"strategy":"priority","worker_count":1,"queue_capacity":1,"scanners":["pii"],"all_groups":true,"endpoints":[]}`)
	require.NoError(t, err)
	require.False(t, cfg.ConversationRecordingEnabled)
	require.False(t, cfg.ConversationReviewEnabled)
	require.Equal(t, DefaultConversationReviewIntervalMinutes, cfg.ConversationReviewIntervalMinutes)
	require.Equal(t, DefaultConversationResponseMaxRunes, cfg.ConversationResponseMaxRunes)
}

func TestBuildConversationWhereKeywordReusesOnePlaceholder(t *testing.T) {
	where, args := buildConversationWhere(ConversationFilter{Keyword: "needle"})
	require.Len(t, args, 1)
	require.NotContains(t, where, "%!")
	require.Equal(t, 2, strings.Count(where, "$1"))
}

func TestClaimConversationCleanupThrottlesHourly(t *testing.T) {
	service := &PromptService{}
	now := time.Date(2026, time.September, 4, 8, 0, 0, 0, time.UTC)
	require.True(t, service.claimConversationCleanup(now))
	require.False(t, service.claimConversationCleanup(now.Add(59*time.Minute)))
	require.True(t, service.claimConversationCleanup(now.Add(time.Hour)))
}
