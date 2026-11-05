package handlers

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/entropy/pkg/ws"
	"github.com/entropy/services/session-manager/labs"
)

// Lab modes.
const (
	modePractice  = "practice"
	modeInterview = "interview"
	modeGameDay   = "gameday"
)

// TypeGameDay carries game day state to the client.
const TypeGameDay = "gameday"

func normalizeMode(m string) string {
	switch m {
	case modeInterview, modeGameDay:
		return m
	}
	return modePractice
}

// interviewScore grades a run from the transcript alone, so it is
// repeatable: time against par, failed commands, and undone fixes.
func interviewScore(seconds, parSeconds, failed, regressions int) (int, string) {
	score := 100
	score -= min(failed*3, 24)
	score -= min(regressions*10, 30)
	if parSeconds > 0 && seconds > parSeconds {
		score -= min((seconds-parSeconds)*30/parSeconds, 30)
	}
	score = max(score, 0)
	grade := "F"
	switch {
	case score >= 90:
		grade = "A"
	case score >= 80:
		grade = "B"
	case score >= 70:
		grade = "C"
	case score >= 60:
		grade = "D"
	}
	return score, grade
}

// helpLocked reports whether hints and AI help are disabled (interview).
func (c *conn) helpLocked() bool {
	c.labMu.Lock()
	defer c.labMu.Unlock()
	return c.lab != nil && c.lab.mode == modeInterview && (c.lab.state == labActive || c.lab.state == labPreparing)
}

// finishInterview scores a completed interview run and asks the AI for
// written feedback on the approach. Called without labMu held.
func (c *conn) finishInterview(run *labRun, seconds int) {
	score, grade := interviewScore(seconds, run.lab.Minutes*60, run.failed, run.regressionCount)
	c.labMu.Lock()
	run.score, run.grade = score, grade
	c.labMu.Unlock()
	_ = c.h.progress.Record(c.ctx, c.owner, labs.Completion{
		LabID: run.lab.ID + ":interview", CompletedAt: time.Now().UTC().Format(time.RFC3339),
		Seconds: seconds, Mode: modeInterview, Score: score, Grade: grade,
	})
	c.recordReplay(replayEvent{Kind: "lab", LabID: run.lab.ID, Message: fmt.Sprintf("interview graded %s (%d/100)", grade, score)})
	question := fmt.Sprintf("Grade my incident response as an interviewer would. I resolved it in %s with %d failed commands and %d "+
		"fixes I later undid; the automatic score is %d/100 (%s). Assess my diagnostic approach from the commands: order of "+
		"investigation, use of evidence, risky actions. Then say what an experienced SRE would have done differently. Under 200 words.",
		formatDuration(seconds), run.failed, run.regressionCount, score, grade)
	context := run.lab.AIContext() + "\n" + c.recentTrail(12)
	go c.streamAI("[interview] "+run.lab.Title, "", 0, false, question, context, "healthy", false)
}

// tickModes enforces interview deadlines and burns the game day budget.
func (c *conn) tickModes(elapsed time.Duration) {
	c.labMu.Lock()
	run := c.lab
	expired := run != nil && run.mode == modeInterview && run.state == labActive &&
		!run.deadline.IsZero() && time.Now().After(run.deadline)
	c.labMu.Unlock()
	if expired {
		c.labMu.Lock()
		run.state = labFailed
		run.score, run.grade = 0, "F"
		c.lab = nil
		c.labMu.Unlock()
		c.recordReplay(replayEvent{Kind: "lab", LabID: run.lab.ID, Message: "interview time expired"})
		c.notice("\x1b[31m[interview] Time is up.\x1b[0m")
		c.sendLab(run, "Time is up. Review the postmortem, then try again.")
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		go func() {
			defer cancel()
			_, _, _ = c.execQuiet(cleanup, labs.CleanupScript)
		}()
	}
	c.burnGameDay(elapsed)
}

// gameDay is a run of several incidents against one error budget.
type gameDay struct {
	labs      []labs.Lab
	index     int
	budget    float64
	burn      float64 // percent per second while an incident is unresolved
	started   time.Time
	resolved  int
	state     string // running, won, lost
	betweenTo time.Time
}

func (g *gameDay) view() map[string]interface{} {
	titles := make([]string, len(g.labs))
	for i, l := range g.labs {
		titles[i] = l.Title
	}
	return map[string]interface{}{
		"state":          g.state,
		"index":          g.index,
		"total":          len(g.labs),
		"resolved":       g.resolved,
		"budget":         g.budget,
		"incidents":      titles,
		"elapsedSeconds": int(time.Since(g.started).Seconds()),
	}
}

func (c *conn) sendGameDay() {
	c.labMu.Lock()
	g := c.gd
	var v map[string]interface{}
	if g != nil {
		v = g.view()
	}
	c.labMu.Unlock()
	if v != nil {
		c.send(TypeGameDay, v)
	}
}

// pickGameDay chooses three incidents. Stack labs are only used when the
// sandbox has python3.
func pickGameDay(all []labs.Lab, stackOK bool, r *rand.Rand) []labs.Lab {
	var pool []labs.Lab
	for _, l := range all {
		if l.Kind != labs.KindIncident || l.Starter || (l.Stack && !stackOK) {
			continue
		}
		pool = append(pool, l)
	}
	r.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	if len(pool) > 3 {
		pool = pool[:3]
	}
	return pool
}

func (c *conn) startGameDay() {
	_, code, _ := c.execQuiet(c.ctx, `python3 -c 'import http.server' 2>/dev/null`)
	picked := pickGameDay(labs.All(), code == 0, rand.New(rand.NewSource(time.Now().UnixNano())))
	if len(picked) == 0 {
		c.send(ws.TypeError, map[string]string{"message": "no incidents available for a game day"})
		return
	}
	par := 0
	for _, l := range picked {
		par += l.Minutes * 60
	}
	g := &gameDay{labs: picked, budget: 100, burn: 100 / (1.2 * float64(par)), started: time.Now(), state: "running"}
	c.labMu.Lock()
	c.gd = g
	c.labMu.Unlock()
	c.recordReplay(replayEvent{Kind: "lab", Message: "game day started: " + strings.Join(titlesOf(picked), ", ")})
	c.sendGameDay()
	c.notice(fmt.Sprintf("\x1b[1;31m[PAGE]\x1b[0m Game day started: %d incidents, one error budget. First page incoming.", len(picked)))
	c.startLab(picked[0].ID, modeGameDay)
}

func titlesOf(ls []labs.Lab) []string {
	out := make([]string, len(ls))
	for i, l := range ls {
		out[i] = l.Title
	}
	return out
}

// gameDayResolved advances to the next incident or ends the game.
func (c *conn) gameDayResolved() {
	c.labMu.Lock()
	g := c.gd
	if g == nil || g.state != "running" {
		c.labMu.Unlock()
		return
	}
	g.resolved++
	g.index++
	done := g.index >= len(g.labs)
	var next labs.Lab
	if done {
		g.state = "won"
	} else {
		next = g.labs[g.index]
		g.betweenTo = time.Now().Add(4 * time.Second)
	}
	c.labMu.Unlock()
	c.sendGameDay()
	if done {
		c.endGameDay()
		return
	}
	go func() {
		time.Sleep(4 * time.Second)
		c.labMu.Lock()
		still := c.gd == g && g.state == "running"
		c.labMu.Unlock()
		if still {
			c.notice(fmt.Sprintf("\x1b[1;31m[PAGE]\x1b[0m %s", next.Summary))
			c.startLab(next.ID, modeGameDay)
		}
	}()
}

func (c *conn) burnGameDay(elapsed time.Duration) {
	c.labMu.Lock()
	g := c.gd
	if g == nil || g.state != "running" || time.Now().Before(g.betweenTo) || c.lab == nil || c.lab.state != labActive {
		c.labMu.Unlock()
		return
	}
	g.budget -= g.burn * elapsed.Seconds()
	lost := g.budget <= 0
	if lost {
		g.budget = 0
		g.state = "lost"
	}
	c.labMu.Unlock()
	c.sendGameDay()
	if lost {
		c.notice("\x1b[31m[game day] The error budget is exhausted.\x1b[0m")
		c.abandonLab("The error budget ran out. Game over.", true)
		c.endGameDay()
	}
}

// endGameDay records the result when the game is won or lost.
func (c *conn) endGameDay() {
	c.labMu.Lock()
	g := c.gd
	c.labMu.Unlock()
	if g == nil {
		return
	}
	if g.state == "running" {
		c.labMu.Lock()
		g.state = "lost"
		c.labMu.Unlock()
		c.sendGameDay()
	}
	seconds := int(time.Since(g.started).Seconds())
	score := int(g.budget)
	if g.state == "won" {
		_ = c.h.progress.Record(c.ctx, c.owner, labs.Completion{
			LabID: "gameday", CompletedAt: time.Now().UTC().Format(time.RFC3339),
			Seconds: seconds, Mode: modeGameDay, Score: score,
		})
	}
	c.recordReplay(replayEvent{Kind: "lab", Message: fmt.Sprintf("game day %s: %d of %d resolved, %d%% budget left", g.state, g.resolved, len(g.labs), score)})
	c.labMu.Lock()
	if c.gd == g {
		c.gd = nil
	}
	c.labMu.Unlock()
}
