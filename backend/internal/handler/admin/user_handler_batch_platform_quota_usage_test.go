package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type batchPlatformQuotaUsageRepoStub struct {
	records   map[int64]*service.UserPlatformQuotaRecord
	snapshots []service.UserPlatformQuotaSnapshot
}

func (s *batchPlatformQuotaUsageRepoStub) GetByUserPlatform(_ context.Context, userID int64, platform string) (*service.UserPlatformQuotaRecord, error) {
	if s.records == nil {
		return nil, nil
	}
	rec := s.records[userID]
	if rec == nil || rec.Platform != platform {
		return nil, nil
	}
	copy := *rec
	return &copy, nil
}

func (s *batchPlatformQuotaUsageRepoStub) BulkInsertInitial(context.Context, []service.UserPlatformQuotaRecord) error {
	panic("unexpected BulkInsertInitial call")
}

func (s *batchPlatformQuotaUsageRepoStub) IncrementUsageWithReset(context.Context, int64, string, float64, time.Time) error {
	panic("unexpected IncrementUsageWithReset call")
}

func (s *batchPlatformQuotaUsageRepoStub) ListByUser(context.Context, int64) ([]service.UserPlatformQuotaRecord, error) {
	panic("unexpected ListByUser call")
}

func (s *batchPlatformQuotaUsageRepoStub) UpsertForUser(context.Context, int64, []service.UserPlatformQuotaRecord) error {
	panic("unexpected UpsertForUser call")
}

func (s *batchPlatformQuotaUsageRepoStub) ResetExpiredWindow(context.Context, int64, string, string, time.Time) error {
	panic("unexpected ResetExpiredWindow call")
}

func (s *batchPlatformQuotaUsageRepoStub) BatchSnapshotUsage(_ context.Context, snapshots []service.UserPlatformQuotaSnapshot, _ time.Time) error {
	s.snapshots = append(s.snapshots, snapshots...)
	return nil
}

func setupBatchPlatformQuotaUsageRouter(serviceStub service.AdminService, repo service.UserPlatformQuotaRepository) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewUserHandler(serviceStub, nil, repo, nil, nil, nil, nil)
	router.POST("/api/v1/admin/users/batch-platform-quota-usage", handler.BatchAdjustPlatformQuotaUsage)
	return router
}

func postBatchPlatformQuotaUsage(t *testing.T, router *gin.Engine, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/admin/users/batch-platform-quota-usage",
		bytes.NewReader(body),
	)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestUserHandlerBatchAdjustPlatformQuotaUsageWritesSnapshots(t *testing.T) {
	require.NoError(t, timezone.Init("Asia/Shanghai"))
	t.Cleanup(func() { _ = timezone.Init("UTC") })

	monthStart := time.Date(2026, 7, 1, 0, 0, 0, 0, timezone.Location())
	repo := &batchPlatformQuotaUsageRepoStub{
		records: map[int64]*service.UserPlatformQuotaRecord{
			4: {
				UserID:             4,
				Platform:           "openai",
				DailyUsageUSD:      0.5,
				WeeklyUsageUSD:     1.5,
				MonthlyUsageUSD:    9,
				MonthlyWindowStart: &monthStart,
			},
		},
	}
	router := setupBatchPlatformQuotaUsageRouter(newStubAdminService(), repo)

	recorder := postBatchPlatformQuotaUsage(
		t,
		router,
		[]byte(`{"user_ids":[4,7],"platform":"openai","daily_usage_usd":1.2,"weekly_usage_usd":3.4}`),
	)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Len(t, repo.snapshots, 2)
	require.Equal(t, int64(4), repo.snapshots[0].UserID)
	require.Equal(t, "openai", repo.snapshots[0].Platform)
	require.Equal(t, 1.2, repo.snapshots[0].DailyUsageUSD)
	require.Equal(t, 3.4, repo.snapshots[0].WeeklyUsageUSD)
	require.Equal(t, 9.0, repo.snapshots[0].MonthlyUsageUSD)
	require.True(t, repo.snapshots[0].MonthlyWindowStart.Equal(monthStart))
	require.Equal(t, 7, repo.snapshots[0].DailyWindowStart.In(timezone.Location()).Hour())
	require.Equal(t, 7, repo.snapshots[0].WeeklyWindowStart.In(timezone.Location()).Hour())

	var response struct {
		Data struct {
			Affected          int       `json:"affected"`
			Platform          string    `json:"platform"`
			DailyWindowStart  time.Time `json:"daily_window_start"`
			WeeklyWindowStart time.Time `json:"weekly_window_start"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, 2, response.Data.Affected)
	require.Equal(t, "openai", response.Data.Platform)
	require.Equal(t, 7, response.Data.DailyWindowStart.In(timezone.Location()).Hour())
	require.Equal(t, 7, response.Data.WeeklyWindowStart.In(timezone.Location()).Hour())
}

func TestUserHandlerBatchAdjustPlatformQuotaUsageRejectsInvalidRequests(t *testing.T) {
	tooManyIDs := make([]int64, 501)
	for index := range tooManyIDs {
		tooManyIDs[index] = int64(index + 1)
	}
	tooManyBody, err := json.Marshal(map[string]any{
		"user_ids":        tooManyIDs,
		"platform":        "openai",
		"daily_usage_usd": 1,
	})
	require.NoError(t, err)

	tests := []struct {
		name string
		body []byte
	}{
		{name: "invalid platform", body: []byte(`{"user_ids":[1],"platform":"unknown","daily_usage_usd":1}`)},
		{name: "missing usage fields", body: []byte(`{"user_ids":[1],"platform":"openai"}`)},
		{name: "negative daily usage", body: []byte(`{"user_ids":[1],"platform":"openai","daily_usage_usd":-1}`)},
		{name: "missing user ids", body: []byte(`{"platform":"openai","daily_usage_usd":1}`)},
		{name: "more than 500 ids", body: tooManyBody},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &batchPlatformQuotaUsageRepoStub{}
			recorder := postBatchPlatformQuotaUsage(
				t,
				setupBatchPlatformQuotaUsageRouter(newStubAdminService(), repo),
				test.body,
			)

			require.Equal(t, http.StatusBadRequest, recorder.Code)
			require.Empty(t, repo.snapshots)
		})
	}
}
