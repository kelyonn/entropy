package handlers

import (
	"math/rand"
	"testing"

	"github.com/entropy/services/session-manager/labs"
)

func TestInterviewScore(t *testing.T) {
	cases := []struct {
		seconds, par, failed, regressions int
		score                             int
		grade                             string
	}{
		{300, 600, 0, 0, 100, "A"},
		{300, 600, 2, 0, 94, "A"},
		{300, 600, 20, 0, 76, "C"},
		{300, 600, 0, 1, 90, "A"},
		{1200, 600, 0, 0, 70, "C"},
		{6000, 600, 20, 9, 16, "F"},
		{900, 600, 3, 1, 66, "D"},
	}
	for _, c := range cases {
		score, grade := interviewScore(c.seconds, c.par, c.failed, c.regressions)
		if score != c.score || grade != c.grade {
			t.Fatalf("interviewScore(%d,%d,%d,%d) = %d %s, want %d %s", c.seconds, c.par, c.failed, c.regressions, score, grade, c.score, c.grade)
		}
	}
}

func TestPickGameDay(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 20; i++ {
		picked := pickGameDay(labs.All(), false, r)
		if len(picked) != 3 {
			t.Fatalf("picked %d incidents", len(picked))
		}
		seen := map[string]bool{}
		for _, l := range picked {
			if l.Starter || l.Stack || l.Kind != labs.KindIncident || seen[l.ID] {
				t.Fatalf("bad pick %+v", l.ID)
			}
			seen[l.ID] = true
		}
	}
	withStack := 0
	for i := 0; i < 50; i++ {
		for _, l := range pickGameDay(labs.All(), true, r) {
			if l.Stack {
				withStack++
			}
		}
	}
	if withStack == 0 {
		t.Fatal("stack labs never picked when available")
	}
}

func TestNormalizeMode(t *testing.T) {
	if normalizeMode("interview") != modeInterview || normalizeMode("") != modePractice || normalizeMode("hack") != modePractice {
		t.Fatal("normalizeMode")
	}
}
