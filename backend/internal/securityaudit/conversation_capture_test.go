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
