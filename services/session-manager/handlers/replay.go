package handlers

import (
	"context"
	"encoding/json"
	"time"
)

// replayEvent is one entry in a session's recorded timeline. The timeline
// backs the postmortem and the shareable replay.
type replayEvent struct {
	At         string `json:"at"`
	Kind       string `json:"kind"` // command, crash, lab, objective, regression, hint, ai
	Command    string `json:"command,omitempty"`
	ExitCode   int    `json:"exitCode,omitempty"`
	DurationMs int64  `json:"durationMs,omitempty"`
	Output     string `json:"output,omitempty"`
	Message    string `json:"message,omitempty"`
	LabID      string `json:"labId,omitempty"`
}

const (
	replayMaxEvents = 2000
	replayTTL       = 30 * 24 * time.Hour
)

func replayKey(sessionID string) string { return "replay:" + sessionID }

func (c *conn) recordReplay(ev replayEvent) {
	if ev.At == "" {
		ev.At = time.Now().UTC().Format(time.RFC3339Nano)
	}
	raw, err := json.Marshal(ev)
	if err != nil {
		return
	}
	c.replayMu.Lock()
	defer c.replayMu.Unlock()
	ctx := context.Background()
	key := replayKey(c.sessionID)
	pipe := c.h.rdb.Pipeline()
	pipe.RPush(ctx, key, raw)
	pipe.LTrim(ctx, key, -replayMaxEvents, -1)
	pipe.Expire(ctx, key, replayTTL)
	_, _ = pipe.Exec(ctx)
}
