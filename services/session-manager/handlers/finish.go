package handlers

import (
	"fmt"
	"strings"
	"time"

	"github.com/entropy/pkg/ws"
)

// finishedCommand is one completed learner or demo command, from either the
// line-mode exec path or the interactive terminal.
type finishedCommand struct {
	command  string
	display  string
	exitCode int
	output   string
	stdout   string
	stderr   string
	duration time.Duration
	cwd      string
	// learner is false for demo scripts, which do not count toward lab
	// objectives or the learner trail.
	learner bool
	// pty marks commands from the interactive terminal, which already showed
	// the prompt and output.
	pty bool
}

// accountCommand records usage for a command that started.
func (c *conn) accountCommand(cmd string) {
	h := c.h
	c.commandsRun.Add(1)
	_ = h.manager.Touch(c.ctx, c.sessionID)
	h.rdb.LPush(c.ctx, fmt.Sprintf("history:%s", c.sessionID), cmd)
	h.rdb.LTrim(c.ctx, fmt.Sprintf("history:%s", c.sessionID), 0, 99)
	h.rdb.Incr(c.ctx, "metric:total_commands")
}

// finishCommand is the shared tail of every command: timeline entry,
// persistence, learner trail, lab objectives, crash handling, and the
// automatic explanation (Requirement 8.1).
func (c *conn) finishCommand(f finishedCommand) {
	h := c.h
	display := f.display
	if display == "" {
		display = f.command
	}
	c.send(ws.TypeOutput, map[string]interface{}{
		"stdout": "", "stderr": "", "exitCode": f.exitCode, "completed": true,
		"command": display, "durationMs": f.duration.Milliseconds(), "cwd": promptPath(f.cwd), "pty": f.pty,
	})
	_ = h.store.InsertCommand(c.ctx, c.sessionID, f.command, f.exitCode, f.stdout, f.stderr)

	output := tail(f.output, 4000)
	interrupted := c.interrupted.Load()
	class := classifyExit(f.exitCode)
	if interrupted && (f.exitCode == 130 || f.exitCode == 143) {
		class = exitFailure
	}
	if f.learner {
		c.recordTrail(f.command, f.exitCode, f.output)
		c.recordLabCommand(f.command)
	}
	c.recordReplay(replayEvent{Kind: "command", Command: display, ExitCode: f.exitCode, DurationMs: f.duration.Milliseconds(), Output: tail(strings.TrimSpace(f.output), 600)})
	switch class {
	case exitCrash:
		c.handleCrash(display, output, f.exitCode, inferEvent(f.exitCode, f.output))
	case exitFailure:
		c.rememberFailure(display, output, f.exitCode)
		c.send(ws.TypeVisualization, map[string]interface{}{"status": "error"})
		// Every non-zero exit is explained automatically, except a command the
		// learner stopped with Ctrl+C and an identical repeat in this session.
		if !interrupted && c.firstAutoExplain(display, f.exitCode) {
			go c.streamAI(display, output, f.exitCode, false, "", c.aiContext(), "error", true)
		}
	default:
		c.send(ws.TypeVisualization, map[string]interface{}{"status": "healthy"})
	}
	go c.sendStats()
	c.afterCommand()
	c.notifyCommandWaiter(commandResult{command: f.command, stdout: f.stdout, stderr: f.stderr, exitCode: f.exitCode})
}
