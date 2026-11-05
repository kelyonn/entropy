package ai

type ExplainRequest struct {
	SessionID string `json:"sessionId"`
	Command   string `json:"command"`
	Stderr    string `json:"stderr"`
	ExitCode  int    `json:"exitCode"`
	Simpler   bool   `json:"simpler,omitempty"`
	Question  string `json:"question,omitempty"`
	// Context is optional grounding, such as the active lab's goals.
	Context string `json:"context,omitempty"`
}

type ExplainToken struct {
	Token    string `json:"token"`
	Done     bool   `json:"done"`
	Fallback bool   `json:"fallback,omitempty"`
}
