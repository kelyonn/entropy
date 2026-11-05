package handlers

import (
	"encoding/base64"
	"strings"
	"testing"
)

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func TestOSCParserSplitsAndStrips(t *testing.T) {
	stream := "ls\r\n\x1b]633;C\x07file1\r\nfile2\r\n\x1b]633;D;2;" + b64("ls /nope") + ";" + b64("/var/log") +
		"\x07\x1b[1;32mroot@entropy\x1b[0m# \x1b]633;A;" + b64("/root") + "\x07"
	// Feed byte by byte to exercise every split point.
	var p oscParser
	var out strings.Builder
	var events []shellEvent
	for i := 0; i < len(stream); i++ {
		o, ev := p.Feed([]byte{stream[i]})
		out.Write(o)
		events = append(events, ev...)
	}
	if strings.Contains(out.String(), "633") {
		t.Fatalf("marker leaked: %q", out.String())
	}
	if !strings.Contains(out.String(), "file2") || !strings.Contains(out.String(), "root@entropy") {
		t.Fatalf("output lost: %q", out.String())
	}
	if len(events) != 3 || events[0].kind != shellCommandStart || events[1].kind != shellCommandDone || events[2].kind != shellPrompt {
		t.Fatalf("events = %+v", events)
	}
	if events[1].exitCode != 2 || events[1].command != "ls /nope" || events[1].cwd != "/var/log" || events[2].cwd != "/root" {
		t.Fatalf("done event = %+v", events[1])
	}
}

func TestOSCParserPassesForeignSequences(t *testing.T) {
	var p oscParser
	out, ev := p.Feed([]byte("\x1b]0;title\x07hello\x1b[31mred"))
	if string(out) != "\x1b]0;title\x07hello\x1b[31mred" || len(ev) != 0 {
		t.Fatalf("foreign sequence altered: %q %v", out, ev)
	}
	out, _ = p.Feed([]byte("\x1b]633;Z;junk\x07after"))
	if string(out) != "after" {
		t.Fatalf("unknown marker not dropped cleanly: %q", out)
	}
}

func TestPlainText(t *testing.T) {
	got := plainText([]byte("\x1b[1;31merror\x1b[0m: no\r\n\x1b]0;t\x07x"))
	if got != "error: no\nx" {
		t.Fatalf("plainText = %q", got)
	}
}

func TestPTYRCInstalled(t *testing.T) {
	if !strings.Contains(installShellRC, "ENTROPY_PTYRC") || !strings.Contains(installShellRC, "PROMPT_COMMAND=__entropy_prompt") {
		t.Fatal("ptyrc not installed")
	}
}

func TestFeedSegmentsKeepsOrder(t *testing.T) {
	var p oscParser
	segs := p.FeedSegments([]byte("cmd\r\n\x1b]633;C\x07out\x1b]633;D;0;" + b64("x") + ";" + b64("/") + "\x07prompt"))
	var kinds []string
	for _, s := range segs {
		if s.event != nil {
			kinds = append(kinds, string(rune(s.event.kind)))
		} else {
			kinds = append(kinds, string(s.data))
		}
	}
	if strings.Join(kinds, "|") != "cmd\r\n|C|out|D|prompt" {
		t.Fatalf("segments out of order: %q", kinds)
	}
}
