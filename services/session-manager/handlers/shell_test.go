package handlers

import (
	"strings"
	"testing"
)

func TestInteractiveHint(t *testing.T) {
	cases := map[string]bool{
		"top":            true,
		"/usr/bin/vim x": true,
		"python3":        true,
		"python3 -c 1":   false,
		"ls -la":         false,
		"  ":             false,
		"ssh host":       true,
		"perl":           true,
	}
	for cmd, want := range cases {
		if got := interactiveHint(cmd) != ""; got != want {
			t.Fatalf("interactiveHint(%q) = %v, want %v", cmd, got, want)
		}
	}
	if !strings.Contains(interactiveHint("perl"), "-e") {
		t.Fatal("perl hint should mention -e")
	}
}

func TestPromptPath(t *testing.T) {
	for in, want := range map[string]string{"/root": "~", "/root/a": "~/a", "/var/log\n": "/var/log", "": "~", "/rootless": "/rootless"} {
		if got := promptPath(in); got != want {
			t.Fatalf("promptPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWrapUserCommandKeepsCommandVerbatim(t *testing.T) {
	cmd := ":(){ :|:& };:"
	w := wrapUserCommand(cmd)
	if !strings.HasSuffix(w, cmd+"\n") || !strings.HasPrefix(w, ". "+shellRC) {
		t.Fatalf("unexpected wrapper:\n%s", w)
	}
	if !strings.Contains(installShellRC, "trap __entropy_save EXIT") || !strings.HasSuffix(installShellRC, "ENTROPY_PTYRC\n") {
		t.Fatalf("rc installer missing prologue:\n%s", installShellRC)
	}
}

func TestClassifyExit(t *testing.T) {
	cases := map[int]exitClass{0: exitOK, 1: exitFailure, 2: exitFailure, 127: exitFailure, 130: exitFailure,
		124: exitCrash, 99: exitCrash, 137: exitCrash, 139: exitCrash, 143: exitCrash}
	for code, want := range cases {
		if got := classifyExit(code); got != want {
			t.Fatalf("classifyExit(%d) = %v, want %v", code, got, want)
		}
	}
}

func TestTrailAndAutoExplainDedupe(t *testing.T) {
	c := &conn{}
	for i := 0; i < 15; i++ {
		c.recordTrail("echo step", 0, "")
	}
	c.recordTrail("kill 42", 1, "bash: kill: (42) - No such process")
	if n := len(c.trail); n != 12 {
		t.Fatalf("trail length %d, want 12", n)
	}
	got := c.recentTrail(2)
	if !strings.Contains(got, "$ kill 42  (exit 1)") || !strings.Contains(got, "No such process") {
		t.Fatalf("trail missing last command:\n%s", got)
	}
	ctx := c.aiContext()
	if !strings.Contains(ctx, "Point out a specific mistake") || !strings.Contains(ctx, "kill 42") {
		t.Fatalf("ai context missing trail:\n%s", ctx)
	}
	if !c.firstAutoExplain("ls x", 2) || c.firstAutoExplain("ls x", 2) || !c.firstAutoExplain("ls x", 1) {
		t.Fatal("auto-explain dedupe wrong")
	}
}
