package handler

import (
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
	"github.com/gin-gonic/gin"
)

// ConversationArchiveMiddleware captures successful model output only when an
// administrator has explicitly enabled conversation recording for the API key's group.
func ConversationArchiveMiddleware(recorder securityaudit.ConversationRecorder) gin.HandlerFunc {
	return func(c *gin.Context) {
		if recorder == nil || !isConversationRequestPath(c.Request) {
			c.Next()
			return
		}
		apiKey := getOpsAPIKey(c)
		var groupID *int64
		if apiKey != nil {
			groupID = apiKey.GroupID
		}
		settings := recorder.ConversationCaptureSettings(groupID)
		if !settings.Enabled {
			c.Next()
			return
		}

		originalWriter := c.Writer
		writer := acquireOpsCaptureWriter(originalWriter)
		writer.setContext(c)
		writer.enableConversationCapture(settings.ResponseMaxBytes)
		defer func() {
			if c.Writer == writer {
				c.Writer = originalWriter
			}
			releaseOpsCaptureWriter(writer)
		}()
		c.Writer = writer
		c.Next()

		value, exists := c.Get(conversationAuditRequestContextKey)
		request, ok := value.(securityaudit.Request)
		if !exists || !ok || strings.TrimSpace(request.Stage) != "http" || !isConversationProtocol(request.Protocol) {
			return
		}
		recorder.EnqueueConversationTurn(request, c.Writer.Status(), c.Writer.Header().Get("Content-Type"), writer.capturedConversationBytes())
	}
}

func isConversationRequestPath(request *http.Request) bool {
	if request == nil || request.Method != http.MethodPost || request.URL == nil {
		return false
	}
	path := strings.TrimSuffix(request.URL.Path, "/")
	switch path {
	case "/v1/messages", "/v1/responses", "/v1/chat/completions", "/responses", "/chat/completions",
		"/backend-api/codex/responses", "/antigravity/v1/messages":
		return true
	}
	return strings.Contains(path, "/models/") &&
		(strings.HasSuffix(path, ":generateContent") || strings.HasSuffix(path, ":streamGenerateContent"))
}

func isConversationProtocol(protocol string) bool {
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "openai_chat_completions", "openai_chat", "chat_completions",
		"anthropic_messages", "claude_messages", "messages",
		"openai_responses", "responses", "gemini", "gemini_generate_content":
		return true
	default:
		return false
	}
}
