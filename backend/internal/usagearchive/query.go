package usagearchive

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type SessionFilter struct {
	UserID     *int64
	APIKeyID   *int64
	GroupID    *int64
	Provider   string
	Model      string
	RequestID  string
	SessionKey string
	StartAt    *time.Time
	EndAt      *time.Time
}

type SessionSummary struct {
	ID                   int64     `json:"id"`
	SessionKey           string    `json:"session_key"`
	UserID               int64     `json:"user_id"`
	APIKeyID             int64     `json:"api_key_id"`
	GroupID              *int64    `json:"group_id,omitempty"`
	Provider             string    `json:"provider"`
	Model                string    `json:"model"`
	FirstSeenAt          time.Time `json:"first_seen_at"`
	LastSeenAt           time.Time `json:"last_seen_at"`
	TurnCount            int       `json:"turn_count"`
	ContentBytes         int64     `json:"content_bytes"`
	ContinuityConfidence string    `json:"continuity_confidence"`
}

type SessionPage struct {
	Items    []SessionSummary `json:"items"`
	Total    int64            `json:"total"`
	Page     int              `json:"page"`
	PageSize int              `json:"page_size"`
	Pages    int              `json:"pages"`
}

type MessageView struct {
	ID             int64  `json:"id"`
	Role           string `json:"role"`
	Source         string `json:"source"`
	Ordinal        int    `json:"ordinal"`
	Content        string `json:"content"`
	ContentHash    string `json:"content_hash"`
	ContentBytes   int    `json:"content_bytes"`
	ContentRunes   int    `json:"content_runes"`
	Truncated      bool   `json:"truncated"`
	ContentExpired bool   `json:"content_expired"`
}

type TurnView struct {
	ID                   int64         `json:"id"`
	RequestID            string        `json:"request_id"`
	ParentTurnID         *int64        `json:"parent_turn_id,omitempty"`
	SequenceNo           int           `json:"sequence_no"`
	ConversationID       string        `json:"conversation_id"`
	PreviousResponseID   string        `json:"previous_response_id"`
	UpstreamResponseID   string        `json:"upstream_response_id"`
	Endpoint             string        `json:"endpoint"`
	Protocol             string        `json:"protocol"`
	Provider             string        `json:"provider"`
	Model                string        `json:"model"`
	StatusCode           int           `json:"status_code"`
	DurationMS           *int          `json:"duration_ms,omitempty"`
	FirstTokenMS         *int          `json:"first_token_ms,omitempty"`
	ContinuitySource     string        `json:"continuity_source"`
	ContinuityConfidence string        `json:"continuity_confidence"`
	RequestTruncated     bool          `json:"request_truncated"`
	ResponseTruncated    bool          `json:"response_truncated"`
	CapturedAt           time.Time     `json:"captured_at"`
	Messages             []MessageView `json:"messages"`
}

type SessionDetail struct {
	SessionSummary
	Turns []TurnView `json:"turns"`
}

type ArchiveStats struct {
	Sessions          int64 `json:"sessions"`
	Turns             int64 `json:"turns"`
	Messages          int64 `json:"messages"`
	ContentBytes      int64 `json:"content_bytes"`
	UserMessages      int64 `json:"user_messages"`
	PromptMessages    int64 `json:"prompt_messages"`
	AssistantMessages int64 `json:"assistant_messages"`
}

func (r *PostgreSQLRepository) GetSession(ctx context.Context, id int64) (*SessionDetail, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("usage archive database unavailable")
	}
	var detail SessionDetail
	err := r.db.QueryRowContext(ctx, `SELECT id,session_key,COALESCE(user_id,0),COALESCE(api_key_id,0),group_id,provider,model,first_seen_at,last_seen_at,turn_count,content_bytes,continuity_confidence FROM usage_archive_sessions WHERE id=$1`, id).Scan(&detail.ID, &detail.SessionKey, &detail.UserID, &detail.APIKeyID, &detail.GroupID, &detail.Provider, &detail.Model, &detail.FirstSeenAt, &detail.LastSeenAt, &detail.TurnCount, &detail.ContentBytes, &detail.ContinuityConfidence)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT t.id,t.request_id,t.parent_turn_id,t.sequence_no,t.conversation_id,t.previous_response_id,t.upstream_response_id,t.endpoint,t.protocol,t.provider,t.model,t.status_code,t.duration_ms,t.first_token_ms,t.continuity_source,t.continuity_confidence,t.request_truncated,t.response_truncated,t.captured_at,m.id,m.role,m.source,m.ordinal,m.content,m.content_hash,m.content_bytes,m.content_runes,m.truncated,m.content_expired FROM usage_archive_turns t LEFT JOIN usage_archive_messages m ON m.turn_id=t.id WHERE t.session_id=$1 ORDER BY t.sequence_no,t.id,m.ordinal,m.id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	turns := make([]TurnView, 0, detail.TurnCount)
	byID := make(map[int64]int)
	for rows.Next() {
		var turn TurnView
		var messageID sql.NullInt64
		var message MessageView
		var messageRole, messageSource, messageContent, messageHash sql.NullString
		var messageOrdinal, messageBytes, messageRunes sql.NullInt64
		var messageTruncated, messageExpired sql.NullBool
		if err := rows.Scan(&turn.ID, &turn.RequestID, &turn.ParentTurnID, &turn.SequenceNo, &turn.ConversationID, &turn.PreviousResponseID, &turn.UpstreamResponseID, &turn.Endpoint, &turn.Protocol, &turn.Provider, &turn.Model, &turn.StatusCode, &turn.DurationMS, &turn.FirstTokenMS, &turn.ContinuitySource, &turn.ContinuityConfidence, &turn.RequestTruncated, &turn.ResponseTruncated, &turn.CapturedAt, &messageID, &messageRole, &messageSource, &messageOrdinal, &messageContent, &messageHash, &messageBytes, &messageRunes, &messageTruncated, &messageExpired); err != nil {
			return nil, err
		}
		index, ok := byID[turn.ID]
		if !ok {
			index = len(turns)
			byID[turn.ID] = index
			turns = append(turns, turn)
		}
		if messageID.Valid {
			message.ID = messageID.Int64
			message.Role = messageRole.String
			message.Source = messageSource.String
			message.Ordinal = int(messageOrdinal.Int64)
			message.Content = messageContent.String
			message.ContentHash = messageHash.String
			message.ContentBytes = int(messageBytes.Int64)
			message.ContentRunes = int(messageRunes.Int64)
			message.Truncated = messageTruncated.Bool
			message.ContentExpired = messageExpired.Bool
			turns[index].Messages = append(turns[index].Messages, message)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	detail.Turns = turns
	return &detail, nil
}

func (r *PostgreSQLRepository) DeleteSessionContent(ctx context.Context, id int64) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("usage archive database unavailable")
	}
	result, err := r.db.ExecContext(ctx, `UPDATE usage_archive_messages m SET content='',content_expired=TRUE FROM usage_archive_turns t WHERE t.id=m.turn_id AND t.session_id=$1 AND m.content<>''`, id)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *PostgreSQLRepository) Stats(ctx context.Context) (ArchiveStats, error) {
	if r == nil || r.db == nil {
		return ArchiveStats{}, errors.New("usage archive database unavailable")
	}
	var stats ArchiveStats
	err := r.db.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM usage_archive_sessions),(SELECT COUNT(*) FROM usage_archive_turns),(SELECT COUNT(*) FROM usage_archive_messages),(SELECT COALESCE(SUM(content_bytes),0) FROM usage_archive_messages),(SELECT COUNT(*) FROM usage_archive_messages WHERE role='user'),(SELECT COUNT(*) FROM usage_archive_messages WHERE role IN ('system','developer')),(SELECT COUNT(*) FROM usage_archive_messages WHERE role='assistant')`).Scan(&stats.Sessions, &stats.Turns, &stats.Messages, &stats.ContentBytes, &stats.UserMessages, &stats.PromptMessages, &stats.AssistantMessages)
	return stats, err
}

func NormalizeArchivePage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func (r *PostgreSQLRepository) ListSessions(ctx context.Context, filter SessionFilter, page, pageSize int) (*SessionPage, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("usage archive database unavailable")
	}
	page, pageSize = NormalizeArchivePage(page, pageSize)
	where, args := archiveSessionWhere(filter)
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM usage_archive_sessions s`+where, args...).Scan(&total); err != nil {
		return nil, err
	}
	limitIndex := len(args) + 1
	queryArgs := append(append([]any(nil), args...), pageSize, (page-1)*pageSize)
	rows, err := r.db.QueryContext(ctx, `SELECT s.id,s.session_key,COALESCE(s.user_id,0),COALESCE(s.api_key_id,0),s.group_id,s.provider,s.model,s.first_seen_at,s.last_seen_at,s.turn_count,s.content_bytes,s.continuity_confidence FROM usage_archive_sessions s`+where+fmt.Sprintf(` ORDER BY s.last_seen_at DESC,s.id DESC LIMIT $%d OFFSET $%d`, limitIndex, limitIndex+1), queryArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]SessionSummary, 0, pageSize)
	for rows.Next() {
		var item SessionSummary
		if err := rows.Scan(&item.ID, &item.SessionKey, &item.UserID, &item.APIKeyID, &item.GroupID, &item.Provider, &item.Model, &item.FirstSeenAt, &item.LastSeenAt, &item.TurnCount, &item.ContentBytes, &item.ContinuityConfidence); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	pages := 0
	if total > 0 {
		pages = int((total + int64(pageSize) - 1) / int64(pageSize))
	}
	return &SessionPage{Items: items, Total: total, Page: page, PageSize: pageSize, Pages: pages}, rows.Err()
}

func archiveSessionWhere(filter SessionFilter) (string, []any) {
	conditions := []string{"1=1"}
	args := make([]any, 0, 8)
	add := func(sqlPart string, value any) {
		args = append(args, value)
		conditions = append(conditions, fmt.Sprintf(sqlPart, len(args)))
	}
	if filter.UserID != nil {
		add("s.user_id=$%d", *filter.UserID)
	}
	if filter.APIKeyID != nil {
		add("s.api_key_id=$%d", *filter.APIKeyID)
	}
	if filter.GroupID != nil {
		add("s.group_id=$%d", *filter.GroupID)
	}
	if strings.TrimSpace(filter.Provider) != "" {
		add("s.provider=$%d", strings.TrimSpace(filter.Provider))
	}
	if strings.TrimSpace(filter.Model) != "" {
		add("s.model ILIKE $%d", "%"+strings.TrimSpace(filter.Model)+"%")
	}
	if strings.TrimSpace(filter.SessionKey) != "" {
		add("s.session_key=$%d", strings.TrimSpace(filter.SessionKey))
	}
	if filter.StartAt != nil {
		add("s.last_seen_at >= $%d", filter.StartAt.UTC())
	}
	if filter.EndAt != nil {
		add("s.last_seen_at < $%d", filter.EndAt.UTC())
	}
	if strings.TrimSpace(filter.RequestID) != "" {
		add("EXISTS (SELECT 1 FROM usage_archive_turns t WHERE t.session_id=s.id AND t.request_id=$%d)", strings.TrimSpace(filter.RequestID))
	}
	return " WHERE " + strings.Join(conditions, " AND "), args
}
