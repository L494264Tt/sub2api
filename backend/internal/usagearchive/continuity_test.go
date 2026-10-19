package usagearchive

import (
	"testing"
	"time"
)

func TestResolveContinuityUsesExplicitIdentifiersBeforeFallback(t *testing.T) {
	base := ContinuityInput{SessionKey: SessionKey{UserID: 7, APIKeyID: 9, Provider: "openai"}, CapturedAt: time.Now()}
	result := ResolveContinuity(ContinuityInput{SessionKey: base.SessionKey, ConversationID: "conv-1", PreviousResponseID: "resp-1"})
	if result.Source != ContinuityConversationID || result.Confidence != ConfidenceCertain || result.Independent {
		t.Fatalf("conversation id result = %#v", result)
	}
	if result.ParentTurnKey != "resp-1" {
		t.Fatalf("parent turn key = %q", result.ParentTurnKey)
	}

	result = ResolveContinuity(ContinuityInput{SessionKey: base.SessionKey, HistoryKey: "history-1"})
	if result.Source != ContinuityHistoryKey || result.Confidence != ConfidenceLikely {
		t.Fatalf("history result = %#v", result)
	}
}

func TestResolveContinuityMarksMissingIdentityIndependent(t *testing.T) {
	result := ResolveContinuity(ContinuityInput{SessionKey: SessionKey{UserID: 7, APIKeyID: 9, Provider: "openai"}})
	if !result.Independent || result.Confidence != ConfidenceNone || result.Source != ContinuityIndependent {
		t.Fatalf("independent result = %#v", result)
	}
}
