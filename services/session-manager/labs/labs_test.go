package labs

import (
	"context"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestCatalogIntegrity(t *testing.T) {
	seen := map[string]bool{}
	for _, l := range All() {
		if l.ID == "" || l.Title == "" || l.Summary == "" || l.Briefing == "" {
			t.Fatalf("lab %q missing display fields", l.ID)
		}
		if seen[l.ID] {
			t.Fatalf("duplicate lab id %q", l.ID)
		}
		seen[l.ID] = true
		switch l.Kind {
		case KindIncident:
			if l.Setup == "" || len(l.Objectives) == 0 || len(l.Hints) == 0 || l.Debrief == "" {
				t.Fatalf("incident %q needs setup, objectives, hints, and debrief", l.ID)
			}
			ids := map[string]bool{}
			for _, o := range l.Objectives {
				if o.ID == "" || o.Text == "" || o.Check == "" {
					t.Fatalf("incident %q has incomplete objective", l.ID)
				}
				if ids[o.ID] {
					t.Fatalf("incident %q duplicates objective %q", l.ID, o.ID)
				}
				ids[o.ID] = true
			}
		case KindDemo:
			if l.Demo == "" {
				t.Fatalf("demo %q has no script", l.ID)
			}
		default:
			t.Fatalf("lab %q has unknown kind %q", l.ID, l.Kind)
		}
		if l.HintCount != len(l.Hints) {
			t.Fatalf("lab %q hint count mismatch", l.ID)
		}
		if strings.Contains(l.Setup, "/tmp/") && strings.Contains(l.Setup, "chmod +x /tmp") {
			t.Fatalf("lab %q places executables on noexec /tmp", l.ID)
		}
	}
}

func TestLegacyIDs(t *testing.T) {
	for legacy, want := range map[string]string{"cpu": "cpu-spike", "fork": "fork-bomb", "oom": "oom-kill", "panic": "kill-init"} {
		l, ok := Get(legacy)
		if !ok || l.ID != want {
			t.Fatalf("Get(%q) = %q, %v; want %q", legacy, l.ID, ok, want)
		}
	}
	if _, ok := Get("nope"); ok {
		t.Fatal("unknown id resolved")
	}
}

func TestCheckScriptAndParse(t *testing.T) {
	l, _ := Get("spool-storm")
	script := CheckScript(l)
	for _, o := range l.Objectives {
		if !strings.Contains(script, markerOK+o.ID) {
			t.Fatalf("script missing objective %q", o.ID)
		}
	}
	out := "noise\nENTROPY_CHECK_OK spawner\nENTROPY_CHECK_NO workers\nENTROPY_CHECK_OK unknown\n"
	got := ParseCheckOutput(l, out)
	if !got["spawner"] || got["workers"] || got["headroom"] {
		t.Fatalf("unexpected parse result %v", got)
	}
	if AllMet(l, got) {
		t.Fatal("AllMet true with failing objectives")
	}
	got["workers"], got["headroom"] = true, true
	if !AllMet(l, got) {
		t.Fatal("AllMet false with all objectives met")
	}
}

func TestAIContextDoesNotLeakHints(t *testing.T) {
	for _, l := range All() {
		ctx := l.AIContext()
		for _, h := range l.Hints {
			if strings.Contains(ctx, h) {
				t.Fatalf("lab %q leaks hint into AI context", l.ID)
			}
		}
	}
}

func TestProgressKeepsBestRun(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	p := NewProgress(rdb)
	ctx := context.Background()
	owner := OwnerKey("u1", "s1")
	if owner != "labprogress:user:u1" {
		t.Fatalf("owner key %q", owner)
	}
	_ = p.Record(ctx, owner, Completion{LabID: "zombie-horde", Seconds: 300, HintsUsed: 2})
	_ = p.Record(ctx, owner, Completion{LabID: "zombie-horde", Seconds: 500, HintsUsed: 3})
	_ = p.Record(ctx, owner, Completion{LabID: "zombie-horde", Seconds: 400, HintsUsed: 0})
	all, err := p.List(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if c := all["zombie-horde"]; c.HintsUsed != 0 || c.Seconds != 400 {
		t.Fatalf("best run not kept: %+v", c)
	}
}
