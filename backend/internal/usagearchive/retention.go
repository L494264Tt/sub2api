package usagearchive

import (
	"fmt"
	"time"
)

func ShouldStoreBody(bodyBytes, dailyBudgetBytes, currentBytes int64) bool {
	if bodyBytes < 0 || currentBytes < 0 || dailyBudgetBytes <= 0 {
		return false
	}
	return bodyBytes <= dailyBudgetBytes-currentBytes
}

func RawContentCutoff(now time.Time, retentionDays int) time.Time {
	if retentionDays <= 0 {
		retentionDays = DefaultRetentionPolicy().RawContentRetentionDays
	}
	return now.Add(-time.Duration(retentionDays) * 24 * time.Hour)
}

func MetadataCutoff(now time.Time, retentionDays int) time.Time {
	if retentionDays <= 0 {
		retentionDays = DefaultRetentionPolicy().MetadataRetentionDays
	}
	return now.Add(-time.Duration(retentionDays) * 24 * time.Hour)
}

func ValidateMessageRole(role MessageRole) error {
	switch role {
	case RoleSystem, RoleDeveloper, RoleUser, RoleAssistant, RoleTool, RoleUnknown:
		return nil
	default:
		return fmt.Errorf("unsupported usage archive message role %q", role)
	}
}
