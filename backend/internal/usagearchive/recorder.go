package usagearchive

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

type Recorder struct {
	repo      Repository
	policy    ArchiveRetentionPolicy
	queue     chan CaptureInput
	workers   sync.WaitGroup
	stop      chan struct{}
	closeOnce sync.Once
}

func NewRecorder(repo Repository, policy ArchiveRetentionPolicy) *Recorder {
	if policy.QueueCapacity <= 0 {
		policy.QueueCapacity = DefaultRetentionPolicy().QueueCapacity
	}
	if policy.WorkerCount <= 0 {
		policy.WorkerCount = DefaultRetentionPolicy().WorkerCount
	}
	r := &Recorder{repo: repo, policy: policy, queue: make(chan CaptureInput, policy.QueueCapacity), stop: make(chan struct{})}
	if !policy.Enabled {
		return r
	}
	for i := 0; i < policy.WorkerCount; i++ {
		r.workers.Add(1)
		go r.worker()
	}
	go r.cleanupLoop()
	return r
}

func (r *Recorder) Enqueue(input CaptureInput) {
	if r == nil || r.repo == nil || input.RequestID == "" {
		return
	}
	select {
	case r.queue <- input:
	default:
		// Archive capture is best-effort; billing and the API request remain independent.
	}
}

func (r *Recorder) Stop(ctx context.Context) error {
	if r == nil {
		return nil
	}
	r.closeOnce.Do(func() { close(r.stop) })
	done := make(chan struct{})
	go func() { r.workers.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *Recorder) worker() {
	defer r.workers.Done()
	for {
		select {
		case input := <-r.queue:
			r.record(input)
		case <-r.stop:
			return
		}
	}
}

func (r *Recorder) cleanupLoop() {
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			_, _ = r.repo.Cleanup(ctx, RawContentCutoff(time.Now().UTC(), r.policy.RawContentRetentionDays), MetadataCutoff(time.Now().UTC(), r.policy.MetadataRetentionDays), r.policy.CleanupBatchSize)
			cancel()
		case <-r.stop:
			return
		}
	}
}

func (r *Recorder) record(input CaptureInput) {
	capture, err := BuildCapture(input)
	if err != nil {
		return
	}
	continuity := ResolveContinuity(ContinuityInput{
		SessionKey:         SessionKey{UserID: input.UserID, APIKeyID: input.APIKeyID, Provider: input.Provider, Identity: input.RequestID},
		ConversationID:     input.ConversationID,
		PreviousResponseID: capture.PreviousResponseID,
		UpstreamResponseID: capture.UpstreamResponseID,
		HistoryKey:         capture.HistoryKey,
		CapturedAt:         input.CapturedAt,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = r.repo.Record(ctx, capture, continuity, r.policy)
}

func BuildCapture(input CaptureInput) (UsageCapture, error) {
	requestMessages, err := ExtractMessages(input.Protocol, input.RequestBody)
	if err != nil {
		// Metadata-only records must survive non-JSON and multipart requests.
		requestMessages = nil
	}
	responseMessages := extractResponseMessages(input.Protocol, input.ResponseContentType, input.ResponseBody)
	capture := UsageCapture{
		RequestID: input.RequestID, UsageLogRequestID: input.UsageLogRequestID, UserID: input.UserID,
		APIKeyID: input.APIKeyID, GroupID: input.GroupID, Provider: input.Provider, Endpoint: input.Endpoint,
		Protocol: input.Protocol, Model: input.Model, ConversationID: input.ConversationID,
		RequestBytes: len(input.RequestBody), ResponseBytes: len(input.ResponseBody), StatusCode: input.StatusCode,
		RequestMessages: requestMessages, ResponseMessages: responseMessages,
		RequestTruncated: input.RequestTruncated, ResponseTruncated: input.ResponseTruncated,
		CapturedAt: input.CapturedAt,
	}
	capture.ConversationID, capture.PreviousResponseID = requestIdentity(input.RequestBody)
	capture.UpstreamResponseID = responseIdentity(input.ResponseBody)
	if input.ConversationID != "" {
		capture.ConversationID = input.ConversationID
	}
	for _, message := range requestMessages {
		switch message.Role {
		case RoleSystem:
			capture.SystemPrompts = append(capture.SystemPrompts, PromptSnapshotFromMessage(message))
		case RoleDeveloper:
			capture.DeveloperPrompts = append(capture.DeveloperPrompts, PromptSnapshotFromMessage(message))
		}
	}
	return capture, nil
}

func requestIdentity(body []byte) (conversationID, previousResponseID string) {
	var root map[string]any
	if json.Unmarshal(body, &root) != nil {
		return "", ""
	}
	conversationID = firstString(root, "conversation_id", "session_id", "thread_id")
	if metadata, ok := root["metadata"].(map[string]any); ok && conversationID == "" {
		conversationID = firstString(metadata, "conversation_id", "session_id", "thread_id")
	}
	previousResponseID = firstString(root, "previous_response_id")
	return conversationID, previousResponseID
}

func responseIdentity(body []byte) string {
	var root map[string]any
	if json.Unmarshal(body, &root) == nil {
		if id := firstString(root, "id"); id != "" {
			return id
		}
		if response, ok := root["response"].(map[string]any); ok {
			return firstString(response, "id")
		}
	}
	for _, line := range strings.FieldsFunc(string(body), func(r rune) bool { return r == '\n' || r == '\r' }) {
		line = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if line == "" || line == "[DONE]" {
			continue
		}
		var event map[string]any
		if json.Unmarshal([]byte(line), &event) == nil {
			if id := firstString(event, "id"); id != "" {
				return id
			}
			if response, ok := event["response"].(map[string]any); ok {
				if id := firstString(response, "id"); id != "" {
					return id
				}
			}
		}
	}
	return ""
}

func firstString(root map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := root[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func PromptSnapshotFromMessage(message ArchiveMessage) PromptSnapshot {
	return PromptSnapshot{Role: message.Role, Content: message.Content, ContentHash: message.ContentHash, ContentBytes: message.ContentBytes, ContentRunes: message.ContentRunes, Truncated: message.Truncated}
}

func extractResponseMessages(protocol, contentType string, body []byte) []ArchiveMessage {
	text := strings.TrimSpace(string(body))
	if text == "" {
		return nil
	}
	var contents []string
	if strings.Contains(strings.ToLower(contentType), "text/event-stream") || strings.HasPrefix(text, "data:") {
		for _, line := range strings.FieldsFunc(text, func(r rune) bool { return r == '\n' || r == '\r' }) {
			line = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if line == "" || line == "[DONE]" {
				continue
			}
			var root map[string]any
			if json.Unmarshal([]byte(line), &root) == nil {
				contents = append(contents, responseText(root)...)
			}
		}
	} else {
		var root map[string]any
		if json.Unmarshal(body, &root) == nil {
			contents = responseText(root)
		}
	}
	result := make([]ArchiveMessage, 0, len(contents))
	for index, content := range contents {
		result = append(result, newArchiveMessage(RoleAssistant, SourceUpstreamReply, index, content))
	}
	_ = protocol
	return result
}

func responseText(root map[string]any) []string {
	if root == nil {
		return nil
	}
	var result []string
	if text, ok := root["output_text"].(string); ok {
		result = append(result, text)
	}
	if choices, ok := root["choices"].([]any); ok {
		for _, raw := range choices {
			choice, _ := raw.(map[string]any)
			if message, ok := choice["message"].(map[string]any); ok {
				result = append(result, contentTexts(message["content"])...)
			}
			if delta, ok := choice["delta"].(map[string]any); ok {
				result = append(result, contentTexts(delta["content"])...)
			}
		}
	}
	if response, ok := root["response"].(map[string]any); ok {
		result = append(result, responseText(response)...)
	}
	if content, ok := root["content"]; ok {
		result = append(result, contentTexts(content)...)
	}
	return result
}

func (r *Recorder) String() string {
	if r == nil {
		return "<nil>"
	}
	return fmt.Sprintf("usage archive recorder workers=%d queue=%d", r.policy.WorkerCount, cap(r.queue))
}
