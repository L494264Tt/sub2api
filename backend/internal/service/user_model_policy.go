package service

import (
	"fmt"
	"path"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/tidwall/gjson"
)

const (
	maxUserModelRestrictions = 64
	maxUserModelPatternLen   = 128
)

var ErrUserModelRestricted = infraerrors.Forbidden(
	"USER_MODEL_RESTRICTED",
	"This model or reasoning effort is not allowed for this user.",
)

// NormalizeUserModelRestrictions validates and canonicalizes admin-supplied rules.
func NormalizeUserModelRestrictions(raw []UserModelRestriction) ([]UserModelRestriction, error) {
	if len(raw) > maxUserModelRestrictions {
		return nil, fmt.Errorf("model restrictions cannot exceed %d entries", maxUserModelRestrictions)
	}

	normalized := make([]UserModelRestriction, 0, len(raw))
	seenPatterns := make(map[string]struct{}, len(raw))
	for i, rule := range raw {
		pattern := strings.ToLower(strings.TrimSpace(rule.ModelPattern))
		if pattern == "" {
			return nil, fmt.Errorf("model restriction %d requires a model pattern", i+1)
		}
		if len(pattern) > maxUserModelPatternLen {
			return nil, fmt.Errorf("model restriction %d pattern cannot exceed %d characters", i+1, maxUserModelPatternLen)
		}
		if _, err := path.Match(pattern, "model"); err != nil {
			return nil, fmt.Errorf("model restriction %d has invalid pattern: %w", i+1, err)
		}
		if _, exists := seenPatterns[pattern]; exists {
			return nil, fmt.Errorf("duplicate model restriction pattern %q", pattern)
		}
		seenPatterns[pattern] = struct{}{}

		efforts := make([]string, 0, len(rule.ReasoningEfforts))
		seenEfforts := make(map[string]struct{}, len(rule.ReasoningEfforts))
		for _, rawEffort := range rule.ReasoningEfforts {
			effort := NormalizeMaxReasoningEffort(rawEffort)
			if effort == "" {
				return nil, fmt.Errorf("model restriction %d contains unknown reasoning effort %q", i+1, rawEffort)
			}
			if _, exists := seenEfforts[effort]; exists {
				continue
			}
			seenEfforts[effort] = struct{}{}
			efforts = append(efforts, effort)
		}
		normalized = append(normalized, UserModelRestriction{
			ModelPattern:     pattern,
			ReasoningEfforts: efforts,
		})
	}
	return normalized, nil
}

// ExtractRequestedOpenAIReasoningEffort returns the explicit request value or
// an effort suffix derived from the requested model name.
func ExtractRequestedOpenAIReasoningEffort(body []byte, model string) string {
	raw := strings.TrimSpace(gjson.GetBytes(body, "reasoning.effort").String())
	if raw == "" {
		raw = strings.TrimSpace(gjson.GetBytes(body, "reasoning_effort").String())
	}
	if raw != "" {
		return NormalizeMaxReasoningEffort(raw)
	}
	return deriveOpenAIReasoningEffortFromModel(model)
}

// CheckUserModelAccess rejects a request when any user rule matches its model
// and, when configured, its requested reasoning effort.
func CheckUserModelAccess(user *User, model string, body []byte) error {
	if user == nil || len(user.ModelRestrictions) == 0 {
		return nil
	}
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" {
		return nil
	}
	baseModel := model
	if slash := strings.LastIndex(baseModel, "/"); slash >= 0 && slash+1 < len(baseModel) {
		baseModel = baseModel[slash+1:]
	}
	effort := ExtractRequestedOpenAIReasoningEffort(body, model)

	for _, rule := range user.ModelRestrictions {
		pattern := strings.ToLower(strings.TrimSpace(rule.ModelPattern))
		matches, err := path.Match(pattern, model)
		if err != nil || !matches {
			matches, _ = path.Match(pattern, baseModel)
		}
		if !matches {
			continue
		}
		if len(rule.ReasoningEfforts) == 0 {
			return ErrUserModelRestricted
		}
		for _, deniedEffort := range rule.ReasoningEfforts {
			if effort != "" && effort == NormalizeMaxReasoningEffort(deniedEffort) {
				return ErrUserModelRestricted
			}
		}
	}
	return nil
}
