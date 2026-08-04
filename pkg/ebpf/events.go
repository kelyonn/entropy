package ebpf

import "time"

const (
	EventOOMKill      = "oom_kill"
	EventKernelPanic  = "kernel_panic"
	EventSegfault     = "segfault"
	EventProcessKill  = "process_kill"
)

type KernelEvent struct {
	SessionID  string                 `json:"sessionId"`
	EventType  string                 `json:"eventType"`
	PID        int                    `json:"pid"`
	Signal     int                    `json:"signal,omitempty"`
	ExitCode   int                    `json:"exitCode,omitempty"`
	Message    string                 `json:"message,omitempty"`
	DetectedAt time.Time              `json:"detectedAt"`
	Raw        map[string]interface{} `json:"raw,omitempty"`
}

type IngestRequest struct {
	Events []KernelEvent `json:"events"`
}
