package ai

type ExplainRequest struct {
	SessionID string `json:"sessionId"`
	Command   string `json:"command"`
	Stderr    string `json:"stderr"`
	ExitCode  int    `json:"exitCode"`
	Simpler   bool   `json:"simpler,omitempty"`
	Question  string `json:"question,omitempty"`
	// Context is optional grounding, such as the active lab's goals and the
	// learner's recent commands.
	Context string `json:"context,omitempty"`
	// Auto marks explanations triggered by a failure rather than requested by
	// the learner. When the hourly budget is spent these get a prepared
	// explanation instead of a rate-limit notice.
	Auto bool `json:"auto,omitempty"`
}

type ExplainToken struct {
	Token    string `json:"token"`
	Done     bool   `json:"done"`
	Fallback bool   `json:"fallback,omitempty"`
}
