package securityaudit

import "time"

const conversationReviewAdvisoryLockKey int64 = 579147893221901923

var ErrConversationNotFound = errorString("conversation review session not found")
var ErrConversationReviewDisabled = errorString("conversation review is disabled")

type errorString string

func (e errorString) Error() string { return string(e) }

type ConversationCaptureSettings struct {
	Enabled          bool
	ResponseMaxBytes int
}

type ConversationRecorder interface {
	ConversationCaptureSettings(groupID *int64) ConversationCaptureSettings
	EnqueueConversationTurn(request Request, statusCode int, contentType string, responseBody []byte, responseTruncated bool)
}

type ConversationSession struct {
	UserPreview            string              `json:"user_preview"`
	ID                     int64               `json:"id"`
	ExternalConversationID string              `json:"external_conversation_id"`
	UserID                 int64               `json:"user_id"`
	UsernameSnapshot       string              `json:"username"`
	UserEmailSnapshot      string              `json:"user_email"`
	APIKeyID               int64               `json:"api_key_id"`
	APIKeyNameSnapshot     string              `json:"api_key_name"`
	GroupID                *int64              `json:"group_id,omitempty"`
	GroupName              string              `json:"group_name"`
	Provider               string              `json:"provider"`
	Protocol               string              `json:"protocol"`
	Model                  string              `json:"model"`
	TurnCount              int                 `json:"turn_count"`
	PendingReviewCount     int                 `json:"pending_review_count"`
	FlaggedTurnCount       int                 `json:"flagged_turn_count"`
	LatestReviewDecision   string              `json:"latest_review_decision"`
	StartedAt              time.Time           `json:"started_at"`
	LastTurnAt             time.Time           `json:"last_turn_at"`
	CreatedAt              time.Time           `json:"created_at"`
	UpdatedAt              time.Time           `json:"updated_at"`
	Turns                  []*ConversationTurn `json:"turns,omitempty"`
}

type ConversationTurn struct {
	ID                 int64                      `json:"id"`
	SessionID          int64                      `json:"session_id"`
	RequestID          string                     `json:"request_id"`
	UpstreamResponseID string                     `json:"upstream_response_id"`
	Endpoint           string                     `json:"endpoint"`
	Protocol           string                     `json:"protocol"`
	Model              string                     `json:"model"`
	RequestKind        string                     `json:"request_kind"`
	RequestDetails     ConversationRequestDetails `json:"request_details"`
	RequestTranscript  string                     `json:"request_transcript"`
	ModelResponse      string                     `json:"model_response"`
	RequestChars       int                        `json:"request_chars"`
	ResponseChars      int                        `json:"response_chars"`
	RequestTruncated   bool                       `json:"request_truncated"`
	ResponseTruncated  bool                       `json:"response_truncated"`
	StatusCode         int                        `json:"status_code"`
	ReviewStatus       string                     `json:"review_status"`
	ReviewAttempts     int                        `json:"review_attempts"`
	ReviewClaimVersion int64                      `json:"-"`
	ReviewStartedAt    *time.Time                 `json:"review_started_at,omitempty"`
	ReviewedAt         *time.Time                 `json:"reviewed_at,omitempty"`
	ReviewDecision     string                     `json:"review_decision"`
	RiskLevel          string                     `json:"risk_level"`
	Action             string                     `json:"action"`
	Categories         []string                   `json:"categories"`
	MatchedScanners    []string                   `json:"matched_scanners"`
	ScannerScores      map[string]float64         `json:"scanner_scores"`
	ScannerEvidence    map[string]string          `json:"scanner_evidence"`
	ScannerBackend     string                     `json:"scanner_backend"`
	ScannerVersion     string                     `json:"scanner_version"`
	GuardEndpointID    string                     `json:"guard_endpoint_id"`
	ReviewErrorCode    string                     `json:"review_error_code"`
	ReviewErrorMessage string                     `json:"review_error_message"`
	CapturedAt         time.Time                  `json:"captured_at"`
	CreatedAt          time.Time                  `json:"created_at"`
	UpdatedAt          time.Time                  `json:"updated_at"`
}

type ConversationRun struct {
	ID               int64      `json:"id"`
	TriggerType      string     `json:"trigger_type"`
	Status           string     `json:"status"`
	RequestedBy      int64      `json:"requested_by"`
	ConfigVersion    int64      `json:"config_version"`
	BatchSize        int        `json:"batch_size"`
	ProcessedCount   int        `json:"processed_count"`
	FlaggedCount     int        `json:"flagged_count"`
	FailedCount      int        `json:"failed_count"`
	StartedAt        *time.Time `json:"started_at,omitempty"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
	LastErrorCode    string     `json:"last_error_code"`
	LastErrorMessage string     `json:"last_error_message"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type ConversationFilter struct {
	RequestKind    string     `json:"request_kind,omitempty"`
	ReviewStatus   string     `json:"review_status,omitempty"`
	Decision       string     `json:"decision,omitempty"`
	GroupID        *int64     `json:"group_id,omitempty"`
	UserID         *int64     `json:"user_id,omitempty"`
	APIKeyID       *int64     `json:"api_key_id,omitempty"`
	ConversationID string     `json:"conversation_id,omitempty"`
	RequestID      string     `json:"request_id,omitempty"`
	Keyword        string     `json:"keyword,omitempty"`
	StartAt        *time.Time `json:"start_at,omitempty"`
	EndAt          *time.Time `json:"end_at,omitempty"`
}

type ConversationPage struct {
	Items    []*ConversationSession `json:"items"`
	Total    int64                  `json:"total"`
	Page     int                    `json:"page"`
	PageSize int                    `json:"page_size"`
	Pages    int                    `json:"pages"`
}

type ConversationRunPage struct {
	Items    []*ConversationRun `json:"items"`
	Total    int64              `json:"total"`
	Page     int                `json:"page"`
	PageSize int                `json:"page_size"`
	Pages    int                `json:"pages"`
}

type ConversationDeleteResult struct {
	DeletedSessions int64 `json:"deleted_sessions"`
	DeletedTurns    int64 `json:"deleted_turns"`
}

type conversationCapture struct {
	RequestKind            string
	RequestDetails         ConversationRequestDetails
	Request                Request
	ConversationKey        string
	ExternalConversationID string
	PreviousResponseID     string
	UpstreamResponseID     string
	RequestTranscript      string
	ModelResponse          string
	RequestChars           int
	ResponseChars          int
	RequestTruncated       bool
	ResponseTruncated      bool
	StatusCode             int
	CapturedAt             time.Time
}

type ConversationMessage struct {
	Role      string `json:"role"`
	Kind      string `json:"kind"`
	Content   string `json:"content"`
	Position  int    `json:"position"`
	Truncated bool   `json:"truncated"`
}

type ConversationRequestDetails struct {
	UserPreview              string                `json:"user_preview,omitempty"`
	Association              string                `json:"association,omitempty"`
	HistoryKey               string                `json:"-"`
	ParentHistoryKey         string                `json:"-"`
	Version                  int                   `json:"version"`
	Messages                 []ConversationMessage `json:"messages"`
	Context                  []ConversationMessage `json:"context"`
	CurrentMessagePosition   int                   `json:"current_message_position"`
	OmittedMessages          int                   `json:"omitted_messages"`
	OmittedContext           int                   `json:"omitted_context"`
	ContextTruncated         bool                  `json:"context_truncated"`
	ResponseContext          []ConversationMessage `json:"response_context"`
	ResponseContextTruncated bool                  `json:"response_context_truncated"`
	ClassificationReason     string                `json:"classification_reason,omitempty"`
}
