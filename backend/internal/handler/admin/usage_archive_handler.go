package admin

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/usagearchive"
	"github.com/gin-gonic/gin"
)

type usageArchiveQueryRepository interface {
	ListSessions(context.Context, usagearchive.SessionFilter, int, int) (*usagearchive.SessionPage, error)
	GetSession(context.Context, int64) (*usagearchive.SessionDetail, error)
	Stats(context.Context) (usagearchive.ArchiveStats, error)
	DeleteSessionContent(context.Context, int64) (int64, error)
}

type UsageArchiveHandler struct{ repo usageArchiveQueryRepository }

func NewUsageArchiveHandler(repo *usagearchive.PostgreSQLRepository) *UsageArchiveHandler {
	return &UsageArchiveHandler{repo: repo}
}

func (h *UsageArchiveHandler) ListSessions(c *gin.Context) {
	if h == nil || h.repo == nil {
		response.Error(c, http.StatusServiceUnavailable, "usage archive unavailable")
		return
	}
	filter := usagearchive.SessionFilter{Provider: strings.TrimSpace(c.Query("provider")), Model: strings.TrimSpace(c.Query("model")), RequestID: strings.TrimSpace(c.Query("request_id")), SessionKey: strings.TrimSpace(c.Query("session_key"))}
	for key, target := range map[string]**int64{"user_id": &filter.UserID, "api_key_id": &filter.APIKeyID, "group_id": &filter.GroupID} {
		if value := strings.TrimSpace(c.Query(key)); value != "" {
			id, err := strconv.ParseInt(value, 10, 64)
			if err != nil || id <= 0 {
				response.BadRequest(c, "invalid "+key)
				return
			}
			*target = &id
		}
	}
	if value := strings.TrimSpace(c.Query("start_at")); value != "" {
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			response.BadRequest(c, "invalid start_at")
			return
		}
		filter.StartAt = &parsed
	}
	if value := strings.TrimSpace(c.Query("end_at")); value != "" {
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			response.BadRequest(c, "invalid end_at")
			return
		}
		filter.EndAt = &parsed
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	result, err := h.repo.ListSessions(c.Request.Context(), filter, page, pageSize)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *UsageArchiveHandler) GetSession(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "invalid session id")
		return
	}
	result, err := h.repo.GetSession(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *UsageArchiveHandler) Stats(c *gin.Context) {
	result, err := h.repo.Stats(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *UsageArchiveHandler) DeleteSessionContent(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "invalid session id")
		return
	}
	deleted, err := h.repo.DeleteSessionContent(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"deleted_messages": deleted})
}
