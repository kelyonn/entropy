package labs

import (
	"encoding/json"
	"fmt"
	"strings"
)

// GeneratorPrompt asks a model for one lab spec. It states the sandbox's
// real constraints and the check helpers, and includes worked examples, so
// most outputs pass static validation on the first try. Feedback carries
// validation problems from a previous attempt.
func GeneratorPrompt(topic, difficulty string, stackOK bool, feedback []string) string {
	example := SpecFromLab(mustGet("zombie-horde"), "pkill -f '^reaper-svc'")
	exampleJSON, _ := json.MarshalIndent(example, "", "  ")
	var community string
	if specs, err := CommunitySpecs(); err == nil && len(specs) > 0 {
		b, _ := json.MarshalIndent(specs[0], "", "  ")
		community = string(b)
	}
	python := "python3 is NOT available; use bash, coreutils, util-linux, procps, and perl."
	if stackOK {
		python = "python3 (full standard library), perl, bash, coreutils, util-linux, procps, iproute2, lsof, and strace are available."
	}

	var b strings.Builder
	fmt.Fprintf(&b, `You design hands-on incident labs for Entropy, a Linux chaos-engineering sandbox.
Write ONE new lab as a single JSON object and nothing else.

Topic: %s
Difficulty: %s

The sandbox:
- Ubuntu 22.04 container, root user, no network at all (loopback only), 256 MB memory, 0.5 CPU, 100 processes.
- %s
- /tmp and /var/log are tmpfs mounted noexec: never put executables there. Put programs under /opt/<lab-name>/ and data under /srv/<lab-name>/.
- Capabilities: only CHOWN, SETUID, SETGID. No mount, no iptables, no kernel modules, no sysctl writes.
- Long-running programs must be started detached: setsid /opt/x/daemon >/dev/null 2>&1 </dev/null &
- Setup runs once as a bash script and must exit 0. It must leave the system BROKEN in a realistic way.

Objectives:
- "check" is a bash snippet that exits 0 when the objective is met. Prefer checking the sandbox state.
- "ran" is a regex over the learner's commands (only for "practice this tool" goals). Use exactly one of check or ran.
- Helper functions available in checks: alive REGEX (a process whose command line matches; write the first character in brackets, e.g. '[o]pt/x/daemon', so the check never matches itself), proccount, zombies, usepct PATH, mempct, as_user USER CMD..., confval FILE KEY.
- "regressed" is shown if a met objective fails again; say what the learner undid.

Rules:
- referenceFix is the shortest correct bash fix. Validation runs setup, requires at least one check to FAIL, runs referenceFix, waits settleSeconds, and requires every objective to PASS. Labs that fail validation are rejected.
- Do not reuse these ids: %s.
- 2 to 4 hints, from gentle to nearly the answer. The debrief explains the root cause and the production lesson in 3 to 5 sentences.
- Calm, technical language. No emoji.

Example of a valid lab (built-in):
%s
`, topic, difficulty, python, strings.Join(allIDs(), ", "), exampleJSON)
	if community != "" {
		fmt.Fprintf(&b, "\nAnother valid lab (community):\n%s\n", community)
	}
	if len(feedback) > 0 {
		b.WriteString("\nYour previous attempt was rejected. Fix every problem:\n")
		for _, f := range feedback {
			b.WriteString("- " + f + "\n")
		}
	}
	b.WriteString("\nReturn only the JSON object with fields: id, title, category, difficulty, minutes, summary, briefing, concepts, objectives, hints, debrief, setup, referenceFix, settleSeconds.\n")
	return b.String()
}

func mustGet(id string) Lab {
	l, _ := Get(id)
	return l
}

func allIDs() []string {
	var ids []string
	for _, l := range All() {
		ids = append(ids, l.ID)
	}
	return ids
}

// ExtractJSON returns the outermost JSON object in a model reply, tolerating
// stray prose or code fences around it.
func ExtractJSON(s string) string {
	start := strings.IndexByte(s, '{')
	end := strings.LastIndexByte(s, '}')
	if start < 0 || end <= start {
		return s
	}
	return s[start : end+1]
}
