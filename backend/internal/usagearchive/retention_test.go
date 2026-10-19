package usagearchive

import (
	"testing"
	"time"
)

func TestShouldStoreBodyHonorsDailyBudget(t *testing.T) {
	if !ShouldStoreBody(100, 1000, 899) {
		t.Fatal("expected body to fit remaining budget")
	}
	if ShouldStoreBody(102, 1000, 899) {
		t.Fatal("expected body to be rejected when budget would be exceeded")
	}
}

func TestRawContentCutoffUsesConfiguredRetention(t *testing.T) {
	now := time.Date(2026, time.October, 19, 12, 0, 0, 0, time.UTC)
	got := RawContentCutoff(now, 7)
	want := now.Add(-7 * 24 * time.Hour)
	if !got.Equal(want) {
		t.Fatalf("RawContentCutoff() = %s, want %s", got, want)
	}
}

func TestValidateMessageRoleAcceptsArchiveRoles(t *testing.T) {
	for _, role := range []MessageRole{RoleSystem, RoleDeveloper, RoleUser, RoleAssistant, RoleTool, RoleUnknown} {
		if err := ValidateMessageRole(role); err != nil {
			t.Fatalf("ValidateMessageRole(%q): %v", role, err)
		}
	}
}
