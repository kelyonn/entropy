package handlers

import (
	"strings"
	"testing"

	"github.com/entropy/services/session-manager/labs"
)

func ev(at, kind, msg string, extra ...func(*replayEvent)) replayEvent {
	e := replayEvent{At: "2026-11-05T10:" + at + "Z", Kind: kind, Message: msg, LabID: "zombie-horde"}
	for _, f := range extra {
		f(&e)
	}
	return e
}

func cmd(c string, code int) func(*replayEvent) {
	return func(e *replayEvent) { e.Command = c; e.ExitCode = code }
}

func TestScopeToLab(t *testing.T) {
	events := []replayEvent{
		{Kind: "command", Command: "before"},
		{Kind: "lab", LabID: "zombie-horde", Message: "started: Zombies"},
		{Kind: "command", Command: "ps"},
		{Kind: "lab", LabID: "zombie-horde", Message: "resolved in 9s with 0 hint(s)"},
		{Kind: "command", Command: "after"},
	}
	got := scopeToLab(events, "zombie-horde")
	if len(got) != 3 || got[1].Command != "ps" {
		t.Fatalf("scope = %+v", got)
	}
	if scopeToLab(events, "missing") != nil {
		t.Fatal("unknown lab scoped")
	}
}

func TestBuildPostmortem(t *testing.T) {
	lab, _ := labs.Get("zombie-horde")
	events := []replayEvent{
		ev("00:00", "lab", "started: Zombies that will not die"),
		ev("00:10", "command", "", cmd("ps aux", 0)),
		ev("00:20", "command", "", cmd("kill -9 48", 0)),
		ev("00:25", "hint", lab.Hints[0]),
		ev("00:40", "command", "", cmd("kill 999", 1)),
		ev("01:00", "objective", "Leave no zombie processes"),
		ev("01:01", "lab", "resolved in 1m 01s with 1 hint(s)"),
	}
	md := BuildPostmortem(events, &lab)
	for _, want := range []string{
		"# Postmortem: Zombies that will not die",
		"**Status:** Resolved in 1m 01s with 1 hint(s)",
		"**Commands run:** 3 (1 failed)",
		"Time to first diagnostic command: 10s",
		"Time to resolve: 1m 01s",
		"| 10:00:40 | kill 999 failed (exit 1) |",
		"## Root cause",
		lab.Debrief,
		"- Leave no zombie processes",
		"- `kill 999` failed with exit 1.",
		"- [ ] Review: wait()",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("postmortem missing %q:\n%s", want, md)
		}
	}
	if !strings.Contains(BuildPostmortem(nil, nil), "No activity") {
		t.Fatal("empty postmortem")
	}
}

func TestPostmortemEscapesPipes(t *testing.T) {
	md := BuildPostmortem([]replayEvent{ev("00:00", "command", "", cmd("ps | grep x", 1))}, nil)
	if !strings.Contains(md, "ps \\| grep x") {
		t.Fatalf("pipe not escaped:\n%s", md)
	}
}
