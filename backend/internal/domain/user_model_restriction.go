package domain

// UserModelRestriction denies a model pattern for all reasoning efforts when
// ReasoningEfforts is empty, or only for the listed efforts otherwise.
type UserModelRestriction struct {
	ModelPattern     string   `json:"model_pattern"`
	ReasoningEfforts []string `json:"reasoning_efforts,omitempty"`
}
