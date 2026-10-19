package usagearchive

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
)

func ResolveContinuity(input ContinuityInput) ContinuityResult {
	if id := strings.TrimSpace(input.ConversationID); id != "" {
		return resultFor(ContinuityConversationID, ConfidenceCertain, id, input.PreviousResponseID)
	}
	if id := strings.TrimSpace(input.PreviousResponseID); id != "" {
		return resultFor(ContinuityPreviousResponse, ConfidenceCertain, id, id)
	}
	if id := strings.TrimSpace(input.UpstreamResponseID); id != "" {
		return resultFor(ContinuityUpstreamResponse, ConfidenceCertain, id, id)
	}
	if id := strings.TrimSpace(input.HistoryKey); id != "" {
		return resultFor(ContinuityHistoryKey, ConfidenceLikely, id, "")
	}
	identity := strings.TrimSpace(input.SessionKey.Identity)
	if identity == "" {
		identity = "independent"
	}
	return ContinuityResult{
		SessionKey:  fallbackSessionKey(input.SessionKey, identity),
		Source:      ContinuityIndependent,
		Confidence:  ConfidenceNone,
		Independent: true,
	}
}

func resultFor(source ContinuitySource, confidence ContinuityConfidence, identity, parent string) ContinuityResult {
	digest := sha256.Sum256([]byte(string(source) + "\x00" + identity))
	return ContinuityResult{
		SessionKey:    hex.EncodeToString(digest[:]),
		ParentTurnKey: parent,
		Source:        source,
		Confidence:    confidence,
	}
}

func fallbackSessionKey(key SessionKey, identity string) string {
	raw := strings.Join([]string{int64String(key.UserID), int64String(key.APIKeyID), key.Provider, identity}, "\x00")
	digest := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(digest[:])
}

func int64String(value int64) string {
	return strconv.FormatInt(value, 10)
}
