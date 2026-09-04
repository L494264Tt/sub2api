package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type userTokenUsageRepoStub struct {
	UsageLogRepository
	usage *UserTokenUsage
	err   error
	calls int
}

func (s *userTokenUsageRepoStub) GetUserTokenUsage(context.Context, int64, time.Time, time.Time) (*UserTokenUsage, error) {
	s.calls++
	return s.usage, s.err
}

func TestCheckBillingEligibilityEnforcesRollingUserTokenLimitsInSimpleMode(t *testing.T) {
	svc := NewBillingCacheService(nil, nil, nil, nil, nil, nil, &config.Config{RunMode: config.RunModeSimple}, nil)
	svc.SetUsageLogRepository(&userTokenUsageRepoStub{usage: &UserTokenUsage{Usage1d: 100, Usage7d: 200, Usage30d: 300}})

	tests := []struct {
		name string
		user *User
		want error
	}{
		{name: "one day", user: &User{ID: 1, TokenLimit1d: 100}, want: ErrUserToken1dQuotaExhausted},
		{name: "seven days", user: &User{ID: 1, TokenLimit7d: 200}, want: ErrUserToken7dQuotaExhausted},
		{name: "thirty days", user: &User{ID: 1, TokenLimit30d: 300}, want: ErrUserToken30dQuotaExhausted},
		{name: "below all limits", user: &User{ID: 1, TokenLimit1d: 101, TokenLimit7d: 201, TokenLimit30d: 301}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := WithRequestedModel(context.Background(), "gpt-5.6-sol")
			err := svc.CheckBillingEligibility(ctx, tt.user, nil, nil, nil, "")
			if tt.want == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tt.want)
		})
	}
}

func TestCheckBillingEligibilitySkipsUserTokenLimitsForNonGPTModels(t *testing.T) {
	repo := &userTokenUsageRepoStub{usage: &UserTokenUsage{Usage1d: 100, Usage7d: 200, Usage30d: 300}}
	svc := NewBillingCacheService(nil, nil, nil, nil, nil, nil, &config.Config{RunMode: config.RunModeSimple}, nil)
	svc.SetUsageLogRepository(repo)
	user := &User{ID: 1, TokenLimit1d: 1, TokenLimit7d: 1, TokenLimit30d: 1}

	for _, model := range []string{"claude-opus-4-6", "gemini-3.1-pro", "grok-4", ""} {
		ctx := WithRequestedModel(context.Background(), model)
		require.NoError(t, svc.CheckBillingEligibility(ctx, user, nil, nil, nil, ""), model)
	}
	require.Zero(t, repo.calls)
}

func TestIsUserTokenQuotaModel(t *testing.T) {
	tests := map[string]bool{
		"gpt-5.6-sol":        true,
		" GPT-5.5 ":          true,
		"openai/gpt-5.6-sol": true,
		"claude-opus-4-6":    false,
		"chatgpt-4o-latest":  false,
		"":                   false,
	}
	for model, want := range tests {
		require.Equal(t, want, isUserTokenQuotaModel(model), model)
	}
}
