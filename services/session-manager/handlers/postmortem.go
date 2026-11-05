package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/entropy/services/session-manager/labs"
	"github.com/redis/go-redis/v9"
)

// ReplayHandler serves recorded session timelines, generated postmortems,
// and share links for replays.
type ReplayHandler struct {
	rdb *redis.Client
}

func NewReplayHandler(rdb *redis.Client) *ReplayHandler {
	return &ReplayHandler{rdb: rdb}
}

func loadReplay(ctx context.Context, rdb *redis.Client, sessionID string) ([]replayEvent, error) {
	raw, err := rdb.LRange(ctx, replayKey(sessionID), 0, -1).Result()
	if err != nil {
		return nil, err
	}
	out := make([]replayEvent, 0, len(raw))
	for _, r := range raw {
		var ev replayEvent
		if json.Unmarshal([]byte(r), &ev) == nil {
			out = append(out, ev)
		}
	}
	return out, nil
}

// scopeToLab returns the events of the most recent run of labID, from its
// start to its resolution or abandonment.
func scopeToLab(events []replayEvent, labID string) []replayEvent {
	start := -1
	for i, ev := range events {
		if ev.Kind == "lab" && ev.LabID == labID && strings.HasPrefix(ev.Message, "started") {
			start = i
		}
	}
	if start < 0 {
		return nil
	}
	end := len(events)
	for i := start + 1; i < len(events); i++ {
		ev := events[i]
		if ev.Kind == "lab" && strings.HasPrefix(ev.Message, "started") {
			end = i
			break
		}
		if ev.Kind == "lab" && ev.LabID == labID {
			end = i + 1
			break
		}
	}
	return events[start:end]
}

func eventTime(ev replayEvent) time.Time {
	t, err := time.Parse(time.RFC3339Nano, ev.At)
	if err != nil {
		return time.Time{}
	}
	return t
}

func mdEscape(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

func clock(d time.Duration) string {
	s := int(d.Seconds())
	if s < 60 {
		return fmt.Sprintf("%ds", s)
	}
	return fmt.Sprintf("%dm %02ds", s/60, s%60)
}

// BuildPostmortem writes a blameless postmortem in Markdown from recorded
// events. It is deterministic so it can be trusted, shared, and tested; the
// AI panel can add narrative on request.
func BuildPostmortem(events []replayEvent, lab *labs.Lab) string {
	var b strings.Builder
	title := "Sandbox session"
	if lab != nil {
		title = lab.Title
	}
	fmt.Fprintf(&b, "# Postmortem: %s\n\n", title)
	if len(events) == 0 {
		b.WriteString("No activity was recorded for this session.\n")
		return b.String()
	}

	start := eventTime(events[0])
	endEv := events[len(events)-1]
	status := "In progress"
	if endEv.Kind == "lab" && strings.HasPrefix(endEv.Message, "resolved") {
		status = "Resolved " + strings.TrimPrefix(endEv.Message, "resolved ")
	} else if endEv.Kind == "lab" && endEv.Message == "abandoned" {
		status = "Abandoned"
	}
	var commands, failed []replayEvent
	var hints, regressions, objectives []replayEvent
	var firstCommand, firstObjective time.Time
	for _, ev := range events {
		switch ev.Kind {
		case "command":
			commands = append(commands, ev)
			if firstCommand.IsZero() {
				firstCommand = eventTime(ev)
			}
			if ev.ExitCode != 0 {
				failed = append(failed, ev)
			}
		case "hint":
			hints = append(hints, ev)
		case "regression":
			regressions = append(regressions, ev)
		case "objective":
			objectives = append(objectives, ev)
			if firstObjective.IsZero() {
				firstObjective = eventTime(ev)
			}
		}
	}

	fmt.Fprintf(&b, "**Status:** %s  \n", status)
	fmt.Fprintf(&b, "**Date:** %s UTC  \n", start.UTC().Format("2006-01-02 15:04"))
	fmt.Fprintf(&b, "**Commands run:** %d (%d failed)  \n", len(commands), len(failed))
	fmt.Fprintf(&b, "**Hints used:** %d\n\n", len(hints))

	b.WriteString("## Summary\n\n")
	if lab != nil {
		b.WriteString(lab.Briefing + "\n\n")
	} else {
		b.WriteString("Free exploration in an isolated sandbox.\n\n")
	}

	b.WriteString("## Key times\n\n")
	if !firstCommand.IsZero() {
		fmt.Fprintf(&b, "- Time to first diagnostic command: %s\n", clock(firstCommand.Sub(start)))
	}
	if !firstObjective.IsZero() {
		fmt.Fprintf(&b, "- Time to first objective met: %s\n", clock(firstObjective.Sub(start)))
	}
	if strings.HasPrefix(status, "Resolved") {
		fmt.Fprintf(&b, "- Time to resolve: %s\n", clock(eventTime(endEv).Sub(start)))
	}
	b.WriteString("\n")

	b.WriteString("## Timeline (UTC)\n\n| Time | Event |\n|---|---|\n")
	rows := 0
	for _, ev := range events {
		if rows >= 80 {
			b.WriteString("| ... | Timeline truncated |\n")
			break
		}
		var text string
		switch ev.Kind {
		case "command":
			// Table cells use escaped plain text: GFM does not honor escaped
			// pipes inside code spans in every renderer.
			text = mdEscape(clip(ev.Command, 120))
			if ev.ExitCode != 0 {
				text += fmt.Sprintf(" failed (exit %d)", ev.ExitCode)
			}
		case "lab":
			text = "Lab " + ev.Message
		case "objective":
			text = "Objective met: " + ev.Message
		case "regression":
			text = "Regression: " + ev.Message
		case "hint":
			text = "Hint used: " + ev.Message
		case "crash":
			text = fmt.Sprintf("Crash: %s", ev.Message)
		case "ai":
			text = "Explanation for " + mdEscape(clip(ev.Command, 80))
		default:
			continue
		}
		if ev.Kind != "command" && ev.Kind != "ai" {
			text = mdEscape(text)
		}
		fmt.Fprintf(&b, "| %s | %s |\n", eventTime(ev).UTC().Format("15:04:05"), text)
		rows++
	}
	b.WriteString("\n")

	b.WriteString("## Root cause\n\n")
	if lab != nil && lab.Debrief != "" {
		b.WriteString(lab.Debrief + "\n\n")
	} else {
		b.WriteString("Not recorded. Describe what failed and why.\n\n")
	}

	b.WriteString("## What went well\n\n")
	wentWell := 0
	for _, o := range objectives {
		fmt.Fprintf(&b, "- %s\n", o.Message)
		wentWell++
	}
	if len(hints) == 0 && strings.HasPrefix(status, "Resolved") {
		b.WriteString("- Resolved without hints.\n")
		wentWell++
	}
	if wentWell == 0 {
		b.WriteString("- Investigation started; see the timeline.\n")
	}
	b.WriteString("\n## What could be improved\n\n")
	improve := 0
	seen := map[string]bool{}
	for _, f := range failed {
		key := fmt.Sprintf("%s|%d", f.Command, f.ExitCode)
		if seen[key] || improve >= 3 {
			continue
		}
		seen[key] = true
		fmt.Fprintf(&b, "- `%s` failed with exit %d.\n", mdEscape(clip(f.Command, 100)), f.ExitCode)
		improve++
	}
	for _, r := range regressions {
		fmt.Fprintf(&b, "- A fix was undone: %s\n", r.Message)
		improve++
	}
	if len(hints) > 0 {
		fmt.Fprintf(&b, "- %d hint(s) were needed; revisit the concepts below.\n", len(hints))
		improve++
	}
	if improve == 0 {
		b.WriteString("- Nothing notable.\n")
	}

	b.WriteString("\n## Action items\n\n")
	if len(regressions) > 0 {
		b.WriteString("- [ ] Re-check earlier objectives after each change; one fix undid another.\n")
	}
	if lab != nil {
		for _, c := range lab.Concepts {
			fmt.Fprintf(&b, "- [ ] Review: %s\n", c)
		}
		if lab.Stack {
			b.WriteString("- [ ] Add an alert on the error-rate and p95 SLOs that fired here.\n")
		}
	}
	b.WriteString("\n_Generated by Entropy from the recorded session timeline._\n")
	return b.String()
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// Postmortem returns a Markdown postmortem for a session, optionally scoped
// to one lab run: GET /api/postmortem?session_id=&lab=
func (h *ReplayHandler) Postmortem(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		http.Error(w, "session_id required", http.StatusBadRequest)
		return
	}
	events, err := loadReplay(r.Context(), h.rdb, sessionID)
	if err != nil {
		http.Error(w, "replay unavailable", http.StatusServiceUnavailable)
		return
	}
	var lab *labs.Lab
	if id := r.URL.Query().Get("lab"); id != "" {
		if l, ok := labs.Get(id); ok {
			lab = &l
			events = scopeToLab(events, l.ID)
		}
	}
	writeJSON(w, map[string]interface{}{"markdown": BuildPostmortem(events, lab), "events": events})
}

// Replay returns recorded events by session id or share token:
// GET /api/replay?session_id= or GET /api/replay?token=
func (h *ReplayHandler) Replay(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := r.URL.Query().Get("session_id")
	if token := r.URL.Query().Get("token"); token != "" {
		id, err := h.rdb.Get(ctx, "replayshare:"+token).Result()
		if err != nil {
			http.Error(w, "replay link not found or expired", http.StatusNotFound)
			return
		}
		sessionID = id
	}
	if sessionID == "" {
		http.Error(w, "session_id or token required", http.StatusBadRequest)
		return
	}
	events, err := loadReplay(ctx, h.rdb, sessionID)
	if err != nil {
		http.Error(w, "replay unavailable", http.StatusServiceUnavailable)
		return
	}
	if id := r.URL.Query().Get("lab"); id != "" {
		events = scopeToLab(events, id)
	}
	writeJSON(w, map[string]interface{}{"events": events})
}

// Share creates a read-only replay link: POST /api/replay/share?session_id=
func (h *ReplayHandler) Share(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		http.Error(w, "session_id required", http.StatusBadRequest)
		return
	}
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		http.Error(w, "could not create link", http.StatusInternalServerError)
		return
	}
	token := hex.EncodeToString(buf)
	if err := h.rdb.Set(r.Context(), "replayshare:"+token, sessionID, replayTTL).Err(); err != nil {
		http.Error(w, "could not create link", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, map[string]string{"token": token})
}
