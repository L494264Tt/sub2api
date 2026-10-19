package admin

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type batchRechargeAdminServiceStub struct {
	*stubAdminService
	calls []batchRechargeCall
}

type batchRechargeCall struct {
	userID int64
	amount float64
	operation string
	notes string
}

func (s *batchRechargeAdminServiceStub) UpdateUserBalance(_ context.Context, userID int64, amount float64, operation, notes string) (*service.User, error) {
	s.calls = append(s.calls, batchRechargeCall{userID: userID, amount: amount, operation: operation, notes: notes})
	return &service.User{ID: userID, Balance: amount, Status: service.StatusActive}, nil
}

func setupBatchRechargeRouter(serviceStub service.AdminService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewUserHandler(serviceStub, nil, nil, nil, nil, nil, nil)
	router.POST("/api/v1/admin/users/batch-recharge", handler.BatchRecharge)
	return router
}

func TestUserHandlerBatchRechargeAddsSameAmountToSelectedUsers(t *testing.T) {
	serviceStub := &batchRechargeAdminServiceStub{stubAdminService: newStubAdminService()}
	router := setupBatchRechargeRouter(serviceStub)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/batch-recharge", bytes.NewBufferString(`{"user_ids":[1,2],"amount":12.5,"notes":"bulk test"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Len(t, serviceStub.calls, 2)
	require.Equal(t, batchRechargeCall{userID: 1, amount: 12.5, operation: "add", notes: "bulk test"}, serviceStub.calls[0])
	require.Equal(t, batchRechargeCall{userID: 2, amount: 12.5, operation: "add", notes: "bulk test"}, serviceStub.calls[1])
}

func TestUserHandlerBatchRechargeRejectsNonPositiveAmount(t *testing.T) {
	serviceStub := &batchRechargeAdminServiceStub{stubAdminService: newStubAdminService()}
	router := setupBatchRechargeRouter(serviceStub)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/batch-recharge", bytes.NewBufferString(`{"user_ids":[1],"amount":0}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Empty(t, serviceStub.calls)
}
