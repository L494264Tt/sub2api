package service

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeUserModelRestrictions(t *testing.T) {
	rules, err := NormalizeUserModelRestrictions([]UserModelRestriction{
		{ModelPattern: " GPT-5.6-SOL* ", ReasoningEfforts: []string{"X-HIGH", "xhigh", "MAX"}},
	})
	require.NoError(t, err)
	require.Equal(t, []UserModelRestriction{
		{ModelPattern: "gpt-5.6-sol*", ReasoningEfforts: []string{"xhigh", "max"}},
	}, rules)
}

func TestCheckUserModelAccess(t *testing.T) {
	user := &User{ModelRestrictions: []UserModelRestriction{
		{ModelPattern: "gpt-5.6-sol", ReasoningEfforts: []string{"xhigh"}},
		{ModelPattern: "gpt-5.6-pro*"},
	}}

	require.ErrorIs(t, CheckUserModelAccess(user, "gpt-5.6-sol", []byte(`{"reasoning":{"effort":"X-HIGH"}}`)), ErrUserModelRestricted)
	require.NoError(t, CheckUserModelAccess(user, "gpt-5.6-sol", []byte(`{"reasoning":{"effort":"high"}}`)))
	require.ErrorIs(t, CheckUserModelAccess(user, "openai/gpt-5.6-pro-preview", []byte(`{}`)), ErrUserModelRestricted)
	require.NoError(t, CheckUserModelAccess(user, "gpt-5.6-mini", []byte(`{}`)))
}

func TestCheckUserModelAccessDerivesEffortFromModelSuffix(t *testing.T) {
	user := &User{ModelRestrictions: []UserModelRestriction{
		{ModelPattern: "gpt-5.6-sol-xhigh", ReasoningEfforts: []string{"xhigh"}},
	}}
	require.True(t, errors.Is(CheckUserModelAccess(user, "gpt-5.6-sol-xhigh", []byte(`{}`)), ErrUserModelRestricted))
}
