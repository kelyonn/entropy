package server

import (
	"strings"
	"testing"
)

func TestComposeOutput(t *testing.T) {
	if got := composeOutput("  boom \n", ""); got != "boom" {
		t.Fatalf("no-context output = %q", got)
	}
	got := composeOutput(strings.Repeat("a", 3000)+"TAIL", "lab goals")
	if !strings.Contains(got, "TAIL") || !strings.HasPrefix(got, "...") {
		t.Fatalf("output tail not kept: %q", got[:40])
	}
	if !strings.HasSuffix(got, "--- Learner context (not program output) ---\nlab goals") {
		t.Fatalf("context not labeled: %q", got[len(got)-80:])
	}
	if got := composeOutput("", "ctx"); !strings.HasPrefix(got, "(no output)") {
		t.Fatalf("empty output = %q", got)
	}
}
