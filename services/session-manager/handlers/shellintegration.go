package handlers

import (
	"bytes"
	"encoding/base64"
	"regexp"
	"strconv"
	"strings"
)

// Shell integration for the interactive terminal. The sandbox shell emits
// private OSC 633 sequences (the same family VS Code uses) around each
// command:
//
//	ESC ] 633 ; C BEL                          command started (PS0)
//	ESC ] 633 ; D ; <exit> ; <b64 cmd> ; <b64 cwd> BEL   command finished
//	ESC ] 633 ; A ; <b64 cwd> BEL              prompt shown, no new command
//
// The Session Manager strips them from the byte stream and turns them into
// events, which keeps exit codes, explanations, lab objectives, and the
// timeline working while the learner uses a real TTY.

var oscPrefix = []byte("\x1b]633;")

const maxPendingOSC = 8192

type shellEventKind byte

const (
	shellCommandStart shellEventKind = 'C'
	shellCommandDone  shellEventKind = 'D'
	shellPrompt       shellEventKind = 'A'
)

type shellEvent struct {
	kind     shellEventKind
	exitCode int
	command  string
	cwd      string
}

type oscParser struct {
	pending []byte
}

// segment is either terminal bytes to forward or a parsed event, in stream
// order, so output can be attributed to the command that produced it.
type segment struct {
	data  []byte
	event *shellEvent
}

// FeedSegments consumes terminal bytes and returns ordered segments with the
// markers removed. Markers split across chunks are held until complete.
func (p *oscParser) FeedSegments(in []byte) []segment {
	data := in
	if len(p.pending) > 0 {
		data = append(p.pending, in...)
		p.pending = nil
	}
	var segs []segment
	emit := func(b []byte) {
		if len(b) > 0 {
			segs = append(segs, segment{data: append([]byte(nil), b...)})
		}
	}
	for len(data) > 0 {
		i := bytes.Index(data, oscPrefix)
		if i < 0 {
			keep := partialPrefixLen(data)
			emit(data[:len(data)-keep])
			if keep > 0 {
				p.pending = append([]byte(nil), data[len(data)-keep:]...)
			}
			break
		}
		emit(data[:i])
		rest := data[i+len(oscPrefix):]
		end := bytes.IndexByte(rest, 0x07)
		if end < 0 {
			if len(data[i:]) > maxPendingOSC {
				emit(data[i:])
			} else {
				p.pending = append([]byte(nil), data[i:]...)
			}
			break
		}
		if ev, ok := parseOSC(string(rest[:end])); ok {
			ev := ev
			segs = append(segs, segment{event: &ev})
		}
		data = rest[end+1:]
	}
	return segs
}

// Feed is FeedSegments flattened into forwarded bytes and events.
func (p *oscParser) Feed(in []byte) ([]byte, []shellEvent) {
	var out bytes.Buffer
	var events []shellEvent
	for _, s := range p.FeedSegments(in) {
		if s.event != nil {
			events = append(events, *s.event)
		} else {
			out.Write(s.data)
		}
	}
	return out.Bytes(), events
}

func partialPrefixLen(data []byte) int {
	max := len(oscPrefix) - 1
	if max > len(data) {
		max = len(data)
	}
	for n := max; n > 0; n-- {
		if bytes.Equal(data[len(data)-n:], oscPrefix[:n]) {
			return n
		}
	}
	return 0
}

func decodeB64(s string) string {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return ""
	}
	return string(b)
}

func parseOSC(payload string) (shellEvent, bool) {
	parts := strings.Split(payload, ";")
	switch parts[0] {
	case "C":
		return shellEvent{kind: shellCommandStart}, true
	case "A":
		ev := shellEvent{kind: shellPrompt}
		if len(parts) > 1 {
			ev.cwd = decodeB64(parts[1])
		}
		return ev, true
	case "D":
		if len(parts) < 4 {
			return shellEvent{}, false
		}
		code, err := strconv.Atoi(parts[1])
		if err != nil {
			return shellEvent{}, false
		}
		return shellEvent{kind: shellCommandDone, exitCode: code, command: decodeB64(parts[2]), cwd: decodeB64(parts[3])}, true
	}
	return shellEvent{}, false
}

var ansiRE = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(\x07|\x1b\\)|\x1b[@-Z\\-_]|\r`)

// plainText removes terminal control sequences from captured output so it
// can be shown to the AI and stored.
func plainText(b []byte) string {
	return ansiRE.ReplaceAllString(string(b), "")
}

// ptyRC is the rc file for the interactive shell. It restores state saved
// by line-mode commands, sets a familiar prompt, and emits the markers.
const ptyRC = `# Entropy interactive shell
[ -r ` + shellStateDir + `/env ] && . ` + shellStateDir + `/env 2>/dev/null
__entropy_cwd=/root
[ -r ` + shellStateDir + `/cwd ] && __entropy_cwd=$(< ` + shellStateDir + `/cwd)
cd "$__entropy_cwd" 2>/dev/null || cd /root
unset __entropy_cwd
HISTCONTROL=
HISTSIZE=2000
HISTFILE=` + shellStateDir + `/history
HISTTIMEFORMAT=
shopt -s histappend checkwinsize
alias ls='ls --color=auto' grep='grep --color=auto'
PS1='\[\e[1;32m\]root@entropy\[\e[0m\]:\[\e[1;34m\]\w\[\e[0m\]# '
PS0=$'\e]633;C\a'
__entropy_last=init
__entropy_b64() { printf %s "$1" | /usr/bin/base64 -w0; }
__entropy_prompt() {
  local ec=$? line n cmd
  history -a 2>/dev/null
  pwd 2>/dev/null > ` + shellStateDir + `/cwd
  export -p 2>/dev/null | /bin/grep -v -E '^declare -x (PWD|OLDPWD|SHLVL|_|ENTROPY_[A-Z_]*|TERM|COLORTERM|COLUMNS|LINES)=' 2>/dev/null > ` + shellStateDir + `/env.next && /bin/mv -f ` + shellStateDir + `/env.next ` + shellStateDir + `/env 2>/dev/null
  line=$(HISTTIMEFORMAT= history 1)
  line="${line#"${line%%[![:space:]]*}"}"
  n=${line%%[[:space:]]*}
  cmd=${line#"$n"}
  cmd="${cmd#"${cmd%%[![:space:]]*}"}"
  if [ "$__entropy_last" = init ]; then
    __entropy_last=$n
    printf '\e]633;A;%s\a' "$(__entropy_b64 "$PWD")"
  elif [ -n "$n" ] && [ "$n" != "$__entropy_last" ]; then
    __entropy_last=$n
    printf '\e]633;D;%s;%s;%s\a' "$ec" "$(__entropy_b64 "$cmd")" "$(__entropy_b64 "$PWD")"
  else
    printf '\e]633;A;%s\a' "$(__entropy_b64 "$PWD")"
  fi
}
PROMPT_COMMAND=__entropy_prompt
`
