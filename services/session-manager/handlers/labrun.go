package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/entropy/pkg/ws"
	"github.com/entropy/services/session-manager/labs"
	"go.uber.org/zap"
)

const (
	labPreparing = "preparing"
	labActive    = "active"
	labCompleted = "completed"
	labAbandoned = "abandoned"
	labFailed    = "failed"
)

// labRun is the state of the lab a connection is working on.
type labRun struct {
	lab       labs.Lab
	startedAt time.Time
	hintsUsed int
	status    map[string]bool
	state     string
	checking  bool
	cancel    context.CancelFunc
}

type objectiveView struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	Done bool   `json:"done"`
}

func (r *labRun) view(message string) map[string]interface{} {
	objectives := make([]objectiveView, 0, len(r.lab.Objectives))
	met := 0
	for _, o := range r.lab.Objectives {
		done := r.status[o.ID]
		if done {
			met++
		}
		objectives = append(objectives, objectiveView{ID: o.ID, Text: o.Text, Done: done})
	}
	v := map[string]interface{}{
		"id":             r.lab.ID,
		"title":          r.lab.Title,
		"kind":           r.lab.Kind,
		"category":       r.lab.Category,
		"difficulty":     r.lab.Difficulty,
		"briefing":       r.lab.Briefing,
		"concepts":       r.lab.Concepts,
		"state":          r.state,
		"active":         r.state == labPreparing || r.state == labActive,
		"objectives":     objectives,
		"objectivesMet":  met,
		"hints":          r.lab.Hints[:r.hintsUsed],
		"hintCount":      len(r.lab.Hints),
		"hintsUsed":      r.hintsUsed,
		"startedAt":      r.startedAt.UTC().Format(time.RFC3339),
		"elapsedSeconds": int(time.Since(r.startedAt).Seconds()),
	}
	if r.state == labCompleted {
		v["debrief"] = r.lab.Debrief
	}
	if message != "" {
		v["message"] = message
	}
	return v
}

func (c *conn) sendLab(r *labRun, message string) {
	c.labMu.Lock()
	v := r.view(message)
	c.labMu.Unlock()
	c.send(ws.TypeLab, v)
}

func (c *conn) handleLabAction(action, id string) {
	switch action {
	case "", "start":
		c.startLab(id)
	case "check":
		go c.checkLab(true)
	case "hint":
		c.revealHint()
	case "abandon", "interrupt":
		c.abandonLab("Lab abandoned. The sandbox was cleaned up.", true)
	default:
		c.send(ws.TypeError, map[string]string{"message": "unknown lab action"})
	}
}

func (c *conn) startLab(id string) {
	lab, ok := labs.Get(id)
	if !ok {
		c.send(ws.TypeError, map[string]string{"message": "unknown lab"})
		return
	}
	c.abandonLab("", false)

	ctx, cancel := context.WithCancel(c.ctx)
	run := &labRun{
		lab: lab, startedAt: time.Now(), status: map[string]bool{},
		state: labPreparing, cancel: cancel,
	}
	c.labMu.Lock()
	c.lab = run
	c.labMu.Unlock()
	c.h.rdb.Incr(c.ctx, "metric:labs_started")
	c.sendLab(run, "Preparing the sandbox.")

	if lab.Kind == labs.KindDemo {
		go c.runDemo(ctx, run)
		return
	}
	go c.prepareIncident(ctx, run)
}

func (c *conn) prepareIncident(ctx context.Context, run *labRun) {
	c.notice(fmt.Sprintf("\x1b[36m[lab] %s\x1b[0m preparing incident...", run.lab.Title))
	_, _, _ = c.execQuiet(ctx, labs.CleanupScript)
	out, code, err := c.execQuiet(ctx, run.lab.Setup)
	if ctx.Err() != nil {
		return
	}
	if err != nil || code != 0 {
		c.h.logger.Warn("lab setup failed", zap.String("lab", run.lab.ID), zap.Int("exit", code), zap.Error(err), zap.String("output", tail(out, 500)))
		c.labMu.Lock()
		run.state = labFailed
		if c.lab == run {
			c.lab = nil
		}
		c.labMu.Unlock()
		c.sendLab(run, "The lab could not be prepared in this sandbox. Try again, or pick another lab.")
		return
	}
	c.labMu.Lock()
	run.state = labActive
	c.labMu.Unlock()
	c.notice(fmt.Sprintf("\x1b[36m[lab] %s\x1b[0m is live. Objectives are in the mission panel.", run.lab.Title))
	c.checkLab(false)
}

func (c *conn) runDemo(ctx context.Context, run *labRun) {
	c.labMu.Lock()
	run.state = labActive
	c.labMu.Unlock()
	c.sendLab(run, "")
	done := make(chan commandResult, 1)
	select {
	case <-ctx.Done():
		return
	case c.cmdQueue <- queuedCommand{command: run.lab.Demo, display: "[demo] " + run.lab.Title, done: done}:
	}
	var result commandResult
	select {
	case <-ctx.Done():
		return
	case result = <-done:
	}
	c.labMu.Lock()
	if c.lab != run {
		c.labMu.Unlock()
		return
	}
	run.state = labCompleted
	c.lab = nil
	c.labMu.Unlock()
	c.sendLab(run, "Demo finished.")
	if classifyExit(result.exitCode) != exitCrash {
		output := result.stdout + result.stderr
		go c.streamAI("[demo] "+run.lab.Title, tail(output, 3000), result.exitCode, false,
			"Walk me through what this demo just showed.", run.lab.AIContext(), "healthy")
	}
}

// afterCommand re-checks objectives after every learner command so progress
// appears without pressing Check.
func (c *conn) afterCommand() {
	c.labMu.Lock()
	run := c.lab
	active := run != nil && run.state == labActive && run.lab.Kind == labs.KindIncident
	c.labMu.Unlock()
	if active {
		go c.checkLab(false)
	}
}

func (c *conn) checkLab(explicit bool) {
	c.labMu.Lock()
	run := c.lab
	if run == nil || run.state != labActive || run.lab.Kind != labs.KindIncident || run.checking {
		c.labMu.Unlock()
		if explicit && run == nil {
			c.send(ws.TypeError, map[string]string{"message": "no lab is running"})
		}
		return
	}
	run.checking = true
	c.labMu.Unlock()

	out, _, err := c.execQuiet(c.ctx, labs.CheckScript(run.lab))

	c.labMu.Lock()
	run.checking = false
	if c.lab != run || run.state != labActive {
		c.labMu.Unlock()
		return
	}
	if err != nil {
		c.labMu.Unlock()
		if explicit {
			c.sendLab(run, "Could not reach the sandbox to check objectives. Try again.")
		}
		return
	}
	run.status = labs.ParseCheckOutput(run.lab, out)
	completed := labs.AllMet(run.lab, run.status)
	if completed {
		run.state = labCompleted
		c.lab = nil
	}
	c.labMu.Unlock()

	if !completed {
		msg := ""
		if explicit {
			met := 0
			for _, ok := range run.status {
				if ok {
					met++
				}
			}
			msg = fmt.Sprintf("%d of %d objectives met.", met, len(run.lab.Objectives))
		}
		c.sendLab(run, msg)
		return
	}

	seconds := int(time.Since(run.startedAt).Seconds())
	c.labsCompleted.Add(1)
	c.h.rdb.Incr(c.ctx, "metric:labs_completed")
	_ = c.h.progress.Record(c.ctx, c.owner, labs.Completion{
		LabID: run.lab.ID, CompletedAt: time.Now().UTC().Format(time.RFC3339),
		Seconds: seconds, HintsUsed: run.hintsUsed,
	})
	c.notice(fmt.Sprintf("\x1b[32m[lab] %s resolved in %s with %d hint(s).\x1b[0m", run.lab.Title, formatDuration(seconds), run.hintsUsed))
	c.sendLab(run, "All objectives met.")
	c.send(ws.TypeVisualization, map[string]interface{}{"status": "healthy"})
}

func (c *conn) revealHint() {
	c.labMu.Lock()
	run := c.lab
	if run == nil || run.state != labActive {
		c.labMu.Unlock()
		c.send(ws.TypeError, map[string]string{"message": "no lab is running"})
		return
	}
	if run.hintsUsed < len(run.lab.Hints) {
		run.hintsUsed++
	}
	c.labMu.Unlock()
	c.sendLab(run, "")
}

// abandonLab stops the active lab. With announce false it is silent, which
// is used when starting another lab or closing the connection.
func (c *conn) abandonLab(message string, announce bool) {
	c.labMu.Lock()
	run := c.lab
	c.lab = nil
	c.labMu.Unlock()
	if run == nil {
		if announce {
			c.send(ws.TypeError, map[string]string{"message": "no lab is running"})
		}
		return
	}
	if run.cancel != nil {
		run.cancel()
	}
	c.labMu.Lock()
	run.state = labAbandoned
	c.labMu.Unlock()
	if run.lab.Kind == labs.KindIncident {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		go func() {
			defer cancel()
			_, _, _ = c.execQuiet(cleanupCtx, labs.CleanupScript)
		}()
	}
	if announce {
		c.sendLab(run, message)
	}
}

func formatDuration(seconds int) string {
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	return fmt.Sprintf("%dm %02ds", seconds/60, seconds%60)
}
