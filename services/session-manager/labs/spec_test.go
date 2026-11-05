package labs

import (
	"strings"
	"testing"
)

func validSpec() Spec {
	return Spec{
		ID: "sample-lab", Title: "Sample", Category: "Processes", Difficulty: "Beginner", Minutes: 5,
		Summary: "s", Briefing: "b", Debrief: "d", Concepts: []string{"signals"},
		Hints:        []string{"one", "two"},
		Setup:        "mkdir -p /opt/sample",
		ReferenceFix: "true",
		Objectives:   []SpecObjective{{ID: "a", Text: "A", Check: "true"}, {ID: "b", Text: "B", Ran: `^ps\b`}},
	}
}

func TestSpecValidate(t *testing.T) {
	if p := validSpec().Validate(); len(p) != 0 {
		t.Fatalf("valid spec rejected: %v", p)
	}
	cases := map[string]func(*Spec){
		"id must be":               func(s *Spec) { s.ID = "Bad ID" },
		"collides with a built-in": func(s *Spec) { s.ID = "zombie-horde" },
		"difficulty":               func(s *Spec) { s.Difficulty = "Easy" },
		"referenceFix is required": func(s *Spec) { s.ReferenceFix = "" },
		"noexec":                   func(s *Spec) { s.Setup = "echo x > /tmp/run.sh && chmod +x /tmp/run.sh" },
		"must not contain":         func(s *Spec) { s.Setup = "kill -9 1" },
		"exactly one of check":     func(s *Spec) { s.Objectives[0].Ran = "x" },
		"invalid ran regex":        func(s *Spec) { s.Objectives[1].Ran = "(" },
		"at least one objective must check": func(s *Spec) {
			s.Objectives = []SpecObjective{{ID: "b", Text: "B", Ran: "ps"}}
		},
		"at least two hints": func(s *Spec) { s.Hints = []string{"one"} },
	}
	for want, mutate := range cases {
		s := validSpec()
		s.Objectives = append([]SpecObjective(nil), s.Objectives...)
		mutate(&s)
		problems := strings.Join(s.Validate(), "; ")
		if !strings.Contains(problems, want) {
			t.Fatalf("expected %q in problems, got %q", want, problems)
		}
	}
}

func TestParseSpecRejectsUnknownFields(t *testing.T) {
	if _, err := ParseSpec([]byte(`{"id":"x","sneaky":true}`)); err == nil {
		t.Fatal("unknown field accepted")
	}
}

func TestCommunitySpecsLoadAndRegister(t *testing.T) {
	specs, err := CommunitySpecs()
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) == 0 {
		t.Fatal("no community specs")
	}
	for _, s := range specs {
		l, ok := Get(s.ID)
		if !ok || !l.Community || l.HintCount != len(s.Hints) {
			t.Fatalf("community lab %s not registered correctly: %+v", s.ID, l)
		}
	}
}

func TestSpecRoundTripFromBuiltin(t *testing.T) {
	l, _ := Get("zombie-horde")
	s := SpecFromLab(l, "pkill -f '^reaper-svc'")
	s.ID = "zombie-copy"
	if p := s.Validate(); len(p) != 0 {
		t.Fatalf("exported built-in fails validation: %v", p)
	}
	back := s.Lab()
	if len(back.Objectives) != len(l.Objectives) || back.Setup != l.Setup {
		t.Fatal("round trip lost data")
	}
}

func TestGeneratorPromptAndExtract(t *testing.T) {
	p := GeneratorPrompt("a stuck cron job", "Beginner", false, []string{"setup must not contain \"kill -9 1\""})
	for _, want := range []string{"a stuck cron job", "python3 is NOT available", "noexec", "zombie-horde", "previous attempt was rejected", "kill -9 1", "referenceFix"} {
		if !strings.Contains(p, want) {
			t.Fatalf("prompt missing %q", want)
		}
	}
	if got := ExtractJSON("sure!\n```json\n{\"a\": {\"b\": 1}}\n```"); got != `{"a": {"b": 1}}` {
		t.Fatalf("ExtractJSON = %q", got)
	}
}
