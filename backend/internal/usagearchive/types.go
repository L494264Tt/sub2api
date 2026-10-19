package usagearchive

import (
	"context"
	"time"
)

type MessageRole string

const (
	RoleSystem    MessageRole = "system"
	RoleDeveloper MessageRole = "developer"
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleTool      MessageRole = "tool"
	RoleUnknown   MessageRole = "unknown"
)

type MessageSource string

const (
	SourceClientRequest MessageSource = "client_request"
	SourceUpstreamReply MessageSource = "upstream_reply"
	SourceContext       MessageSource = "context"
	SourceTool          MessageSource = "tool"
)

type ContinuitySource string

const (
	ContinuityConversationID   ContinuitySource = "conversation_id"
	ContinuityPreviousResponse ContinuitySource = "previous_response_id"
	ContinuityUpstreamResponse ContinuitySource = "upstream_response_id"
	ContinuityHistoryKey       ContinuitySource = "history_key"
	ContinuityFallback         ContinuitySource = "fallback"
	ContinuityIndependent      ContinuitySource = "independent"
)

type ContinuityConfidence string

const (
	ConfidenceCertain ContinuityConfidence = "certain"
	ConfidenceLikely  ContinuityConfidence = "likely"
	ConfidenceLow     ContinuityConfidence = "low"
	ConfidenceNone    ContinuityConfidence = "none"
)

type ArchiveRetentionPolicy struct {
	Enabled                 bool
	RawContentRetentionDays int
	MetadataRetentionDays   int
	DailyContentBudgetBytes int64
	QueueCapacity           int
	WorkerCount             int
	CleanupBatchSize        int
}

func DefaultRetentionPolicy() ArchiveRetentionPolicy {
	return ArchiveRetentionPolicy{
		Enabled:                 true,
		RawContentRetentionDays: 7,
		MetadataRetentionDays:   90,
		DailyContentBudgetBytes: 2 << 30,
		QueueCapacity:           2048,
		WorkerCount:             2,
		CleanupBatchSize:        100,
	}
}

type ArchiveMessage struct {
	Role         MessageRole
	Source       MessageSource
	Ordinal      int
	Content      string
	ContentHash  string
	ContentBytes int
	ContentRunes int
	Truncated    bool
}

type PromptSnapshot struct {
	Role         MessageRole
	Content      string
	ContentHash  string
	ContentBytes int
	ContentRunes int
	Truncated    bool
}

type UsageCapture struct {
	RequestID          string
	UsageLogRequestID  string
	UserID             int64
	APIKeyID           int64
	GroupID            *int64
	Provider           string
	Endpoint           string
	Protocol           string
	Model              string
	ConversationID     string
	PreviousResponseID string
	UpstreamResponseID string
	HistoryKey         string
	CapturedAt         time.Time
	StatusCode         int
	DurationMS         *int
	FirstTokenMS       *int
	RequestMessages    []ArchiveMessage
	ResponseMessages   []ArchiveMessage
	SystemPrompts      []PromptSnapshot
	DeveloperPrompts   []PromptSnapshot
	RequestBytes       int
	ResponseBytes      int
	RequestTruncated   bool
	ResponseTruncated  bool
}

type CaptureInput struct {
	RequestID           string
	UsageLogRequestID   string
	UserID              int64
	APIKeyID            int64
	GroupID             *int64
	Provider            string
	Endpoint            string
	Protocol            string
	Model               string
	ConversationID      string
	RequestBody         []byte
	ResponseBody        []byte
	ResponseContentType string
	StatusCode          int
	CapturedAt          time.Time
	RequestTruncated    bool
	ResponseTruncated   bool
}

type SessionKey struct {
	UserID   int64
	APIKeyID int64
	Provider string
	Identity string
}

type ContinuityInput struct {
	SessionKey         SessionKey
	ConversationID     string
	PreviousResponseID string
	UpstreamResponseID string
	HistoryKey         string
	CapturedAt         time.Time
}

type ContinuityResult struct {
	SessionKey    string
	ParentTurnKey string
	Source        ContinuitySource
	Confidence    ContinuityConfidence
	Independent   bool
}

type Repository interface {
	Record(ctx context.Context, capture UsageCapture, continuity ContinuityResult, policy ArchiveRetentionPolicy) error
	Cleanup(ctx context.Context, rawBefore, metadataBefore time.Time, batchSize int) (CleanupResult, error)
	StorageStats(ctx context.Context) (StorageStats, error)
}

type CleanupResult struct {
	DeletedContentRows int64
	DeletedTurns       int64
	DeletedSessions    int64
}

type StorageStats struct {
	SessionBytes        int64
	TurnBytes           int64
	MessageBytes        int64
	PromptSnapshotBytes int64
	SessionCount        int64
	TurnCount           int64
	MessageCount        int64
	RawContentBytes     int64
}
