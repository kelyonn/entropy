package labs

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// Spec is the portable lab format used by community labs (labs/community)
// and by the AI generator. A spec is only published after Validate passes
// statically and ValidateInSandbox proves it is broken after setup and
// fixed by its reference fix.
type Spec struct {
	ID            string          `json:"id"`
	Title         string          `json:"title"`
	Category      string          `json:"category"`
	Difficulty    string          `json:"difficulty"`
	Minutes       int             `json:"minutes"`
	Summary       string          `json:"summary"`
	Briefing      string          `json:"briefing"`
	Concepts      []string        `json:"concepts"`
	Objectives    []SpecObjective `json:"objectives"`
	Hints         []string        `json:"hints"`
	Debrief       string          `json:"debrief"`
	Setup         string          `json:"setup"`
	ReferenceFix  string          `json:"referenceFix"`
	SettleSeconds int             `json:"settleSeconds,omitempty"`
	Stack         bool            `json:"stack,omitempty"`
}

type SpecObjective struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	Check     string `json:"check,omitempty"`
	Ran       string `json:"ran,omitempty"`
	Regressed string `json:"regressed,omitempty"`
}

var (
	idRE         = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{2,40}$`)
	difficulties = map[string]bool{"Beginner": true, "Intermediate": true, "Advanced": true}
	// Executables must not be created on the noexec mounts.
	noexecExecRE = regexp.MustCompile(`chmod\s+[^\n;&|]*\+?x[^\n;&|]*\s/(tmp|var/log)/`)
	// Setup must not take down the sandbox it is preparing.
	forbiddenSetup = []string{"kill -9 1", "kill 1 ", "reboot", "shutdown", "/proc/sysrq-trigger", ":(){"}
)

// ParseSpec decodes a spec, rejecting unknown fields so typos surface.
func ParseSpec(data []byte) (Spec, error) {
	var s Spec
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&s); err != nil {
		return Spec{}, fmt.Errorf("invalid lab JSON: %w", err)
	}
	return s, nil
}

// Validate checks a spec statically and returns every problem found.
func (s Spec) Validate() []string {
	var p []string
	add := func(format string, args ...interface{}) { p = append(p, fmt.Sprintf(format, args...)) }
	if !idRE.MatchString(s.ID) {
		add("id must be 3-41 lowercase letters, digits, or dashes")
	}
	if _, exists := builtinIDs()[s.ID]; exists {
		add("id %q collides with a built-in lab", s.ID)
	}
	for name, v := range map[string]string{"title": s.Title, "summary": s.Summary, "briefing": s.Briefing, "debrief": s.Debrief, "category": s.Category} {
		if strings.TrimSpace(v) == "" {
			add("%s is required", name)
		}
	}
	if !difficulties[s.Difficulty] {
		add("difficulty must be Beginner, Intermediate, or Advanced")
	}
	if s.Minutes < 2 || s.Minutes > 45 {
		add("minutes must be between 2 and 45")
	}
	if len(s.Concepts) == 0 {
		add("at least one concept is required")
	}
	if len(s.Hints) < 2 {
		add("at least two hints are required")
	}
	if strings.TrimSpace(s.Setup) == "" || len(s.Setup) > 32_000 {
		add("setup is required and must be under 32 KB")
	}
	if strings.TrimSpace(s.ReferenceFix) == "" {
		add("referenceFix is required: validation must prove the lab is solvable")
	}
	if noexecExecRE.MatchString(s.Setup) {
		add("setup makes a file executable under /tmp or /var/log, which are mounted noexec; use /opt or /srv")
	}
	for _, f := range forbiddenSetup {
		if strings.Contains(s.Setup, f) {
			add("setup must not contain %q", f)
		}
	}
	if len(s.Objectives) == 0 || len(s.Objectives) > 6 {
		add("one to six objectives are required")
	}
	ids := map[string]bool{}
	hasCheck := false
	for _, o := range s.Objectives {
		if o.ID == "" || o.Text == "" {
			add("every objective needs an id and text")
		}
		if ids[o.ID] {
			add("duplicate objective id %q", o.ID)
		}
		ids[o.ID] = true
		if (o.Check == "") == (o.Ran == "") {
			add("objective %q needs exactly one of check or ran", o.ID)
		}
		if o.Check != "" {
			hasCheck = true
		}
		if o.Ran != "" {
			if _, err := regexp.Compile(o.Ran); err != nil {
				add("objective %q has an invalid ran regex: %v", o.ID, err)
			}
		}
	}
	if !hasCheck {
		add("at least one objective must check sandbox state")
	}
	if s.SettleSeconds < 0 || s.SettleSeconds > 20 {
		add("settleSeconds must be between 0 and 20")
	}
	return p
}

// Lab converts a validated spec into a runnable lab.
func (s Spec) Lab() Lab {
	l := Lab{
		ID: s.ID, Title: s.Title, Kind: KindIncident, Category: s.Category, Difficulty: s.Difficulty,
		Minutes: s.Minutes, Summary: s.Summary, Briefing: s.Briefing, Concepts: s.Concepts,
		Hints: s.Hints, HintCount: len(s.Hints), Debrief: s.Debrief, Setup: s.Setup, Stack: s.Stack,
		Community: true,
	}
	for _, o := range s.Objectives {
		l.Objectives = append(l.Objectives, Objective{ID: o.ID, Text: o.Text, Check: o.Check, Ran: o.Ran, Regressed: o.Regressed})
	}
	return l
}

// SpecFromLab exports a built-in lab as a spec, used as the generator's
// worked example.
func SpecFromLab(l Lab, fix string) Spec {
	s := Spec{
		ID: l.ID, Title: l.Title, Category: l.Category, Difficulty: l.Difficulty, Minutes: l.Minutes,
		Summary: l.Summary, Briefing: l.Briefing, Concepts: l.Concepts, Hints: l.Hints,
		Debrief: l.Debrief, Setup: l.Setup, ReferenceFix: fix,
	}
	for _, o := range l.Objectives {
		s.Objectives = append(s.Objectives, SpecObjective{ID: o.ID, Text: o.Text, Check: o.Check, Ran: o.Ran, Regressed: o.Regressed})
	}
	return s
}

// Registry of community and generated labs, in addition to the built-ins.
var (
	extraMu sync.RWMutex
	extra   []Lab
)

func builtinIDs() map[string]bool {
	ids := map[string]bool{}
	for _, l := range catalog {
		ids[l.ID] = true
	}
	return ids
}

// Register adds or replaces a community or generated lab.
func Register(l Lab) {
	extraMu.Lock()
	defer extraMu.Unlock()
	for i := range extra {
		if extra[i].ID == l.ID {
			extra[i] = l
			return
		}
	}
	extra = append(extra, l)
}

func extras() []Lab {
	extraMu.RLock()
	defer extraMu.RUnlock()
	out := make([]Lab, len(extra))
	copy(out, extra)
	return out
}
