package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type conversationRecorderStub struct {
	settings  securityaudit.ConversationCaptureSettings
	request   securityaudit.Request
	status    int
	body      []byte
	truncated bool
}

func TestConversationArchiveMiddlewareMarksTruncationWithoutChangingResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const limit = 32
	recorder := &conversationRecorderStub{settings: securityaudit.ConversationCaptureSettings{Enabled: true, ResponseMaxBytes: limit}}
	router := gin.New()
	router.Use(ConversationArchiveMiddleware(recorder))
	body := `{"output_text":"` + strings.Repeat("a", limit*2) + `"}`
	router.POST("/v1/responses", func(c *gin.Context) {
		c.Set(conversationAuditRequestContextKey, securityaudit.Request{Protocol: "openai_responses", Stage: "http"})
		c.Data(http.StatusOK, "application/json", []byte(body))
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/responses", nil))
	require.Equal(t, body, response.Body.String())
	require.Len(t, recorder.body, limit)
	require.True(t, recorder.truncated)
}

func (s *conversationRecorderStub) ConversationCaptureSettings(*int64) securityaudit.ConversationCaptureSettings {
	return s.settings
}

func (s *conversationRecorderStub) EnqueueConversationTurn(request securityaudit.Request, statusCode int, _ string, responseBody []byte, truncated bool) {
	s.request = request
	s.status = statusCode
	s.body = append([]byte(nil), responseBody...)
	s.truncated = truncated
}

func TestConversationArchiveMiddlewareCapturesEnabledSuccessfulResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &conversationRecorderStub{settings: securityaudit.ConversationCaptureSettings{Enabled: true, ResponseMaxBytes: 1024}}
	router := gin.New()
	router.Use(ConversationArchiveMiddleware(recorder))
	router.POST("/v1/responses", func(c *gin.Context) {
		c.Set(conversationAuditRequestContextKey, securityaudit.Request{RequestID: "req-1", Protocol: "openai_responses", Stage: "http"})
		c.JSON(http.StatusOK, gin.H{"output_text": "record me"})
	})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/responses", nil))
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "req-1", recorder.request.RequestID)
	require.Equal(t, http.StatusOK, recorder.status)
	require.Contains(t, string(recorder.body), "record me")
}

func TestConversationArchiveMiddlewareDoesNotBufferWhenDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &conversationRecorderStub{}
	router := gin.New()
	router.Use(ConversationArchiveMiddleware(recorder))
	router.POST("/v1/responses", func(c *gin.Context) {
		c.Set(conversationAuditRequestContextKey, securityaudit.Request{RequestID: "req-1", Protocol: "openai_responses", Stage: "http"})
		c.String(http.StatusOK, "not recorded")
	})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/responses", nil))
	require.Empty(t, recorder.request.RequestID)
	require.Empty(t, recorder.body)
}
