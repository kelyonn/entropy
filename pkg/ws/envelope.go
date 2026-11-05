package ws

import (
	"encoding/json"
	"time"
)

const (
	TypeCommand        = "command"
	TypeOutput         = "output"
	TypeVisualization  = "visualization"
	TypeAIStream       = "ai_stream"
	TypeAI             = "ai"
	TypeHeartbeat      = "heartbeat"
	TypePong           = "pong"
	TypeCrash          = "crash"
	TypeScenario       = "scenario"
	TypeError          = "error"
	TypeSessionSummary = "session_summary"
	TypeLab            = "lab"
	TypeShell          = "shell"
	TypeInterrupt      = "interrupt"
	TypeAIExplain      = "ai_explain"
)

type Envelope struct {
	Type            string          `json:"type"`
	SessionID       string          `json:"sessionId"`
	Timestamp       string          `json:"timestamp,omitempty"`
	Payload         json.RawMessage `json:"payload,omitempty"`
	Command         string          `json:"command,omitempty"`
	SessionIDLegacy string          `json:"session_id,omitempty"`
}

type CommandPayload struct {
	Command       string `json:"command"`
	ExecutionType string `json:"executionType,omitempty"`
}

type OutputPayload struct {
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr"`
	ExitCode  int    `json:"exitCode"`
	Completed bool   `json:"completed"`
}

type VisualizationPayload struct {
	Status string `json:"status,omitempty"`
	Event  string `json:"event,omitempty"`
	CPU    *int   `json:"cpu,omitempty"`
	Memory *int   `json:"memory,omitempty"`
}

type AIStreamPayload struct {
	Token string `json:"token"`
	Done  bool   `json:"done"`
	Reset bool   `json:"reset,omitempty"`
}

type SessionSummaryPayload struct {
	CommandsRun        int    `json:"commandsRun"`
	CrashesEncountered int    `json:"crashesEncountered"`
	DurationSeconds    int    `json:"durationSeconds"`
	Reason             string `json:"reason"`
	LabsCompleted      int    `json:"labsCompleted"`
}

func NewEnvelope(typ, sessionID string, payload interface{}) (Envelope, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{
		Type:      typ,
		SessionID: sessionID,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Payload:   body,
	}, nil
}

func MarshalClientMessage(typ, sessionID string, payload interface{}) ([]byte, error) {
	env, err := NewEnvelope(typ, sessionID, payload)
	if err != nil {
		return nil, err
	}
	var legacy map[string]interface{}
	_ = json.Unmarshal(env.Payload, &legacy)
	msg := map[string]interface{}{
		"type":      typ,
		"sessionId": sessionID,
		"payload":   legacy,
		"timestamp": env.Timestamp,
	}
	if v, ok := legacy["data"]; ok {
		msg["data"] = v
	}
	if v, ok := legacy["token"]; ok {
		msg["data"] = v
	}
	return json.Marshal(msg)
}
