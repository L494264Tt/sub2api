package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
	"github.com/Wei-Shaw/sub2api/internal/usagearchive"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ConversationArchiveMiddleware captures successful model output only when an
// administrator has explicitly enabled conversation recording for the API key's group.
func ConversationArchiveMiddleware(recorder securityaudit.ConversationRecorder, usageRecorders ...*usagearchive.Recorder) gin.HandlerFunc {
	return func(c *gin.Context) {
		var usageRecorder *usagearchive.Recorder
		if len(usageRecorders) > 0 {
			usageRecorder = usageRecorders[0]
		}
		if recorder == nil && usageRecorder == nil {
			c.Next()
			return
		}
		apiKey := getOpsAPIKey(c)
		var groupID *int64
		if apiKey != nil {
			groupID = apiKey.GroupID
		}
		settings := securityaudit.ConversationCaptureSettings{}
		if recorder != nil {
			settings = recorder.ConversationCaptureSettings(groupID)
		}
		capturePromptAudit := recorder != nil && settings.Enabled && isConversationRequestPath(c.Request)
		captureUsage := usageRecorder != nil && c.Request != nil && c.Request.Method == http.MethodPost
		if !capturePromptAudit && !captureUsage {
			c.Next()
			return
		}

		originalWriter := c.Writer
		writer := acquireOpsCaptureWriter(originalWriter)
		writer.setContext(c)
		responseMaxBytes := settings.ResponseMaxBytes
		if captureUsage && responseMaxBytes < 4<<20 {
			responseMaxBytes = 4 << 20
		}
		writer.enableConversationCapture(responseMaxBytes)
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
		if !exists || !ok {
			if !captureUsage {
				return
			}
			request = buildUsageArchiveFallbackRequest(c)
		}
		if strings.TrimSpace(request.Stage) != "" && strings.TrimSpace(request.Stage) != "http" {
			return
		}
		responseBody := writer.capturedConversationBytes()
		responseTruncated := writer.conversationCaptureTruncated()
		if capturePromptAudit && isConversationProtocol(request.Protocol) {
			recorder.EnqueueConversationTurn(request, c.Writer.Status(), c.Writer.Header().Get("Content-Type"), responseBody, responseTruncated)
		}
		if captureUsage {
			usageRecorder.Enqueue(usagearchive.CaptureInput{
				RequestID: request.RequestID, UsageLogRequestID: request.RequestID, UserID: request.UserID,
				APIKeyID: request.APIKeyID, GroupID: request.GroupID, Provider: request.Provider,
				Endpoint: request.Endpoint, Protocol: request.Protocol, Model: request.Model,
				ConversationID: request.ConversationID, RequestBody: request.Body, ResponseBody: responseBody,
				ResponseContentType: c.Writer.Header().Get("Content-Type"), StatusCode: c.Writer.Status(),
				CapturedAt: time.Now().UTC(), ResponseTruncated: responseTruncated,
			})
		}
	}
}

func buildUsageArchiveFallbackRequest(c *gin.Context) securityaudit.Request {
	requestID, _ := c.Request.Context().Value(ctxkey.ClientRequestID).(string)
	if strings.TrimSpace(requestID) == "" {
		requestID = strings.TrimSpace(c.GetHeader("X-Request-ID"))
	}
	if strings.TrimSpace(requestID) == "" {
		requestID = uuid.NewString()
	}
	request := securityaudit.Request{
		RequestID: requestID,
		Endpoint:  c.Request.URL.Path,
		Protocol:  "unknown",
		Stage:     "http",
	}
	if apiKey := getOpsAPIKey(c); apiKey != nil {
		request.APIKeyID = apiKey.ID
		request.APIKeyName = apiKey.Name
		request.UserID = apiKey.UserID
		request.GroupID = apiKey.GroupID
		if apiKey.Group != nil {
			request.GroupName = apiKey.Group.Name
			request.Provider = apiKey.Group.Platform
		}
		if apiKey.User != nil {
			request.Username = apiKey.User.Username
			request.UserEmail = apiKey.User.Email
		}
	}
	return request
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
