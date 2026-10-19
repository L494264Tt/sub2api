package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type batchLimitsAdminServiceStub struct {
	*stubAdminService
	calls []batchLimitsAdminServiceCall
}

type batchLimitsAdminServiceCall struct {
	userIDs         []int64
	concurrency     *int
	rpmLimit        *int
	tokenLimit1d    *int64
	tokenLimit7d    *int64
	tokenLimit30d   *int64
	resetTokenQuota bool
}

func cloneIntPointer(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func (s *batchLimitsAdminServiceStub) BatchUpdateLimits(_ context.Context, userIDs []int64, concurrency, rpmLimit *int, tokenLimit1d, tokenLimit7d, tokenLimit30d *int64, resetTokenQuota bool) (int, error) {
	s.calls = append(s.calls, batchLimitsAdminServiceCall{
		userIDs:         append([]int64(nil), userIDs...),
		concurrency:     cloneIntPointer(concurrency),
		rpmLimit:        cloneIntPointer(rpmLimit),
		tokenLimit1d:    cloneInt64Pointer(tokenLimit1d),
		tokenLimit7d:    cloneInt64Pointer(tokenLimit7d),
		tokenLimit30d:   cloneInt64Pointer(tokenLimit30d),
		resetTokenQuota: resetTokenQuota,
	})
	return len(userIDs), nil
}

func cloneInt64Pointer(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func setupBatchLimitsRouter(serviceStub service.AdminService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewUserHandler(serviceStub, nil, nil, nil, nil, nil, nil)
	router.POST("/api/v1/admin/users/batch-limits", handler.BatchUpdateLimits)
	return router
}

func postBatchLimits(t *testing.T, router *gin.Engine, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/admin/users/batch-limits",
		bytes.NewReader(body),
	)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestUserHandlerBatchUpdateLimitsAcceptsPartialAndZeroValues(t *testing.T) {
	tests := []struct {
		name                    string
		body                    string
		expectedConcurrency     *int
		expectedRPMLimit        *int
		expectedTokenLimit1d    *int64
		expectedTokenLimit7d    *int64
		expectedTokenLimit30d   *int64
		expectedResetTokenQuota bool
	}{
		{name: "concurrency only", body: `{"user_ids":[1,2],"concurrency":10}`, expectedConcurrency: pointerTo(10)},
		{name: "both limits", body: `{"user_ids":[1,2],"concurrency":8,"rpm_limit":60}`, expectedConcurrency: pointerTo(8), expectedRPMLimit: pointerTo(60)},
		{name: "explicit zero", body: `{"user_ids":[1,2],"concurrency":0,"rpm_limit":0}`, expectedConcurrency: pointerTo(0), expectedRPMLimit: pointerTo(0)},
		{name: "GPT token limits", body: `{"user_ids":[1,2],"token_limit_1d":100000000,"token_limit_7d":400000000,"token_limit_30d":0}`, expectedTokenLimit1d: pointerToInt64(100000000), expectedTokenLimit7d: pointerToInt64(400000000), expectedTokenLimit30d: pointerToInt64(0)},
		{name: "reset GPT token quota only", body: `{"user_ids":[1,2],"reset_token_quota":true}`, expectedResetTokenQuota: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			serviceStub := &batchLimitsAdminServiceStub{stubAdminService: newStubAdminService()}
			recorder := postBatchLimits(t, setupBatchLimitsRouter(serviceStub), []byte(test.body))

			require.Equal(t, http.StatusOK, recorder.Code)
			require.Len(t, serviceStub.calls, 1)
			require.Equal(t, []int64{1, 2}, serviceStub.calls[0].userIDs)
			require.Equal(t, test.expectedConcurrency, serviceStub.calls[0].concurrency)
			require.Equal(t, test.expectedRPMLimit, serviceStub.calls[0].rpmLimit)
			require.Equal(t, test.expectedTokenLimit1d, serviceStub.calls[0].tokenLimit1d)
			require.Equal(t, test.expectedTokenLimit7d, serviceStub.calls[0].tokenLimit7d)
			require.Equal(t, test.expectedTokenLimit30d, serviceStub.calls[0].tokenLimit30d)
			require.Equal(t, test.expectedResetTokenQuota, serviceStub.calls[0].resetTokenQuota)

			var response struct {
				Data struct {
					Affected int `json:"affected"`
				} `json:"data"`
			}
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
			require.Equal(t, 2, response.Data.Affected)
		})
	}
}

func TestUserHandlerBatchUpdateLimitsRejectsInvalidRequests(t *testing.T) {
	tooManyIDs := make([]int64, 501)
	for index := range tooManyIDs {
		tooManyIDs[index] = int64(index + 1)
	}
	tooManyBody, err := json.Marshal(map[string]any{"user_ids": tooManyIDs, "rpm_limit": 10})
	require.NoError(t, err)

	tests := []struct {
		name string
		body []byte
	}{
		{name: "no limits", body: []byte(`{"user_ids":[1]}`)},
		{name: "invalid json", body: []byte(`{"user_ids":`)},
		{name: "missing user ids", body: []byte(`{"rpm_limit":10}`)},
		{name: "negative token limit", body: []byte(`{"user_ids":[1],"token_limit_1d":-1}`)},
		{name: "more than 500 ids", body: tooManyBody},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			serviceStub := &batchLimitsAdminServiceStub{stubAdminService: newStubAdminService()}
			recorder := postBatchLimits(t, setupBatchLimitsRouter(serviceStub), test.body)

			require.Equal(t, http.StatusBadRequest, recorder.Code)
			require.Empty(t, serviceStub.calls)
		})
	}
}

func TestUserHandlerBatchUpdateLimitsAllUsesEveryListedUser(t *testing.T) {
	base := newStubAdminService()
	base.users = []service.User{{ID: 11}, {ID: 12}, {ID: 13}}
	serviceStub := &batchLimitsAdminServiceStub{stubAdminService: base}
	recorder := postBatchLimits(
		t,
		setupBatchLimitsRouter(serviceStub),
		[]byte(`{"all":true,"user_ids":[999],"rpm_limit":0}`),
	)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Len(t, serviceStub.calls, 1)
	require.Equal(t, []int64{11, 12, 13}, serviceStub.calls[0].userIDs)
	require.Equal(t, 1, base.lastListUsers.calls)
}

func pointerTo(value int) *int {
	return &value
}

func pointerToInt64(value int64) *int64 {
	return &value
}
