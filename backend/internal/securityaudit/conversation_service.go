package securityaudit

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

const conversationCleanupInterval = time.Hour

func (s *PromptService) ConversationCaptureSettings(groupID *int64) ConversationCaptureSettings {
	if s == nil || s.config == nil {
		return ConversationCaptureSettings{}
	}
	cfg, ok := s.config.Active()
	if !ok || !cfg.ConversationRecordingEnabled || !cfg.IncludesGroup(groupID) {
		return ConversationCaptureSettings{}
	}
	limit := cfg.ConversationResponseMaxRunes*utf8.UTFMax + 64*1024
	maximum := MaxConversationCaptureRunes*utf8.UTFMax + 64*1024
	if limit > maximum {
		limit = maximum
	}
	return ConversationCaptureSettings{Enabled: true, ResponseMaxBytes: limit}
}

func (s *PromptService) RecordConversationTurn(ctx context.Context, request Request, statusCode int, contentType string, responseBody []byte) error {
	if s == nil || s.config == nil || s.repo == nil || statusCode < 200 || statusCode >= 300 {
		return nil
	}
	cfg, ok := s.config.Active()
	if !ok || !cfg.ConversationRecordingEnabled || !cfg.IncludesGroup(request.GroupID) {
		return nil
	}
	snapshot, err := ExtractPromptSnapshot(request)
	if errors.Is(err, ErrNoPromptText) {
		return nil
	}
	if err != nil {
		return err
	}
	responseText := normalizeAssistantResponse(request.Protocol, responseBody)
	if strings.TrimSpace(responseText) == "" {
		return nil
	}
	requestText := strings.ReplaceAll(snapshot.ScanText, promptAuditPrioritySeparator, "\n\n")
	requestText, requestChars, requestTruncated := trimConversationText(requestText, cfg.ConversationRequestMaxRunes)
	responseText, responseChars, responseTruncated := trimConversationText(responseText, cfg.ConversationResponseMaxRunes)
	externalID, previousResponseID, responseID := conversationIdentity(request, responseBody)
	capture := conversationCapture{
		Request: request.Clone(), ConversationKey: conversationKey(request, externalID, previousResponseID),
		ExternalConversationID: externalID, PreviousResponseID: previousResponseID, UpstreamResponseID: responseID,
		RequestTranscript: requestText, ModelResponse: responseText, RequestChars: requestChars, ResponseChars: responseChars,
		RequestTruncated: requestTruncated, ResponseTruncated: responseTruncated, StatusCode: statusCode, CapturedAt: s.clock.Now(),
	}
	_ = contentType
	return s.repo.RecordConversationCapture(ctx, capture)
}

func (s *PromptService) EnqueueConversationTurn(request Request, statusCode int, contentType string, responseBody []byte) {
	if s == nil {
		return
	}
	s.lifecycleMu.Lock()
	background := s.background
	s.lifecycleMu.Unlock()
	if background == nil {
		return
	}
	if s.enqueueSlots == nil {
		return
	}
	select {
	case s.enqueueSlots <- struct{}{}:
	default:
		LogWarn("conversation_review.capture_dropped", map[string]any{"request_id": request.RequestID, "status": "dropped", "error_code": "conversation_capture_queue_full"})
		return
	}
	requestCopy := request.Clone()
	responseCopy := append([]byte(nil), responseBody...)
	s.enqueueWG.Add(1)
	go func() {
		defer s.enqueueWG.Done()
		defer func() { <-s.enqueueSlots }()
		ctx, cancel := context.WithTimeout(background, 5*time.Second)
		defer cancel()
		if err := s.RecordConversationTurn(ctx, requestCopy, statusCode, contentType, responseCopy); err != nil {
			LogWarn("conversation_review.capture_failed", map[string]any{"request_id": requestCopy.RequestID, "status": "failed", "error_code": "conversation_capture_failed"})
		}
	}()
}

func (s *PromptService) QueueConversationReview(ctx context.Context, requestedBy int64) (*ConversationRun, error) {
	cfg, ok := s.config.Active()
	if !ok || !cfg.ConversationRecordingEnabled || !cfg.ConversationReviewEnabled || !cfg.Enabled {
		return nil, ErrConversationReviewDisabled
	}
	_ = s.repo.ReclaimStaleConversationReviews(ctx, s.clock.Now().Add(-10*time.Minute))
	return s.repo.QueueConversationReviewRun(ctx, "manual", requestedBy, cfg)
}

func (s *PromptService) ListConversationSessions(ctx context.Context, filter ConversationFilter, page, pageSize int) (*ConversationPage, error) {
	return s.repo.ListConversationSessions(ctx, filter, page, pageSize)
}

func (s *PromptService) GetConversationSession(ctx context.Context, id int64) (*ConversationSession, error) {
	return s.repo.GetConversationSession(ctx, id)
}

func (s *PromptService) DeleteConversationSession(ctx context.Context, id int64) (*ConversationDeleteResult, error) {
	return s.repo.DeleteConversationSession(ctx, id)
}

func (s *PromptService) ListConversationReviewRuns(ctx context.Context, page, pageSize int) (*ConversationRunPage, error) {
	return s.repo.ListConversationReviewRuns(ctx, page, pageSize)
}

func (s *PromptService) conversationReviewLoop(ctx context.Context) {
	defer s.enqueueWG.Done()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runConversationReviewTick(ctx)
		}
	}
}

func (s *PromptService) runConversationReviewTick(ctx context.Context) {
	cfg, ok := s.config.Active()
	if !ok || s.repo == nil {
		return
	}
	now := s.clock.Now()
	if s.claimConversationCleanup(now) {
		if err := s.repo.CleanupConversationReview(ctx, now.AddDate(0, 0, -cfg.ConversationRetentionDays)); err != nil {
			LogWarn("conversation_review.cleanup_failed", map[string]any{"status": "failed", "error_code": "conversation_review_cleanup_failed"})
		}
	}
	if !cfg.Enabled || !cfg.ConversationRecordingEnabled || !cfg.ConversationReviewEnabled || len(cfg.EnabledEndpoints()) == 0 {
		return
	}
	if err := s.repo.ReclaimStaleConversationReviews(ctx, now.Add(-10*time.Minute)); err != nil {
		LogWarn("conversation_review.reclaim_failed", map[string]any{"status": "failed", "error_code": "conversation_review_reclaim_failed"})
		return
	}
	if err := s.repo.ScheduleConversationReviewIfDue(ctx, cfg, now); err != nil {
		LogWarn("conversation_review.schedule_failed", map[string]any{"status": "failed", "error_code": "conversation_review_schedule_failed"})
		return
	}
	run, claimed, err := s.repo.ClaimConversationReviewRun(ctx)
	if err != nil || !claimed {
		return
	}
	processed, flagged, failed, runErr := s.processConversationReviewRun(ctx, cfg, run)
	_ = s.repo.FinishConversationReviewRun(ctx, run.ID, processed, flagged, failed, runErr)
}

func (s *PromptService) claimConversationCleanup(now time.Time) bool {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	if !s.lastConversationCleanupAt.IsZero() && now.Before(s.lastConversationCleanupAt.Add(conversationCleanupInterval)) {
		return false
	}
	s.lastConversationCleanupAt = now
	return true
}

func (s *PromptService) processConversationReviewRun(ctx context.Context, cfg ActiveConfig, run *ConversationRun) (processed, flagged, failed int, runErr error) {
	turns, err := s.repo.ClaimConversationReviewTurns(ctx, run.BatchSize)
	if err != nil {
		return 0, 0, 0, err
	}
	endpoints := cfg.EnabledEndpoints()
	for _, turn := range turns {
		input := "User conversation:\n" + turn.RequestTranscript + "\n\nModel response:\n" + turn.ModelResponse
		chunks := SplitRunes(input, minimumInputLimit(endpoints))
		started := s.clock.Now()
		results := make([]*NormalizedResult, 0, len(chunks))
		var scanErr error
		for _, chunk := range chunks {
			result, err := scanWithFailover(ctx, s.scanner, cfg.Scanners, endpoints, chunk, s.metrics)
			if err != nil {
				scanErr = err
				break
			}
			results = append(results, result)
			if result.Action == ActionBlock {
				break
			}
		}
		if scanErr != nil {
			failed++
			_ = s.repo.FailConversationReviewTurn(ctx, turn, guardErrorCode(scanErr))
			continue
		}
		aggregated, err := AggregateResults(results, s.clock.Now().Sub(started))
		if err != nil {
			failed++
			_ = s.repo.FailConversationReviewTurn(ctx, turn, ErrorCodeInvalidResponse)
			continue
		}
		aggregated.ChunkTotal = len(chunks)
		if err := s.repo.CompleteConversationReviewTurn(ctx, turn, aggregated); err != nil {
			failed++
			continue
		}
		processed++
		if aggregated.Decision != EventPass {
			flagged++
		}
	}
	return processed, flagged, failed, nil
}
