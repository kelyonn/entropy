package handlers

import (
	"strings"
)

// Each command runs in a fresh exec, so shell state would normally vanish
// between commands. The wrapper restores the working directory and exported
// variables from the previous command and saves them again on exit, which
// makes `cd` and `export` behave the way learners expect.
const shellStateDir = "/root/.entropy"

const shellPrologue = `
mkdir -p ` + shellStateDir + ` 2>/dev/null
if [ -r ` + shellStateDir + `/env ]; then . ` + shellStateDir + `/env 2>/dev/null; fi
__entropy_cwd=/root
if [ -r ` + shellStateDir + `/cwd ]; then __entropy_cwd=$(< ` + shellStateDir + `/cwd); fi
cd "$__entropy_cwd" 2>/dev/null || cd /root 2>/dev/null || cd /
unset __entropy_cwd
__entropy_save() {
  pwd 2>/dev/null > ` + shellStateDir + `/cwd
  export -p 2>/dev/null | grep -v -E '^declare -x (PWD|OLDPWD|SHLVL|_|ENTROPY_[A-Z_]*)=' 2>/dev/null > ` + shellStateDir + `/env.next &&
    mv -f ` + shellStateDir + `/env.next ` + shellStateDir + `/env 2>/dev/null
}
trap __entropy_save EXIT
`

// cwdProbe prints the persisted working directory for the prompt.
const cwdProbe = `cat ` + shellStateDir + `/cwd 2>/dev/null || echo /root`

// shellRC is where the prologue lives inside the sandbox. Sourcing a file
// keeps learner commands readable in `ps` output, which matters in labs that
// are about reading the process list.
const shellRC = shellStateDir + "/rc"

// installShellRC writes the prologue into the sandbox. It runs once per
// connection and is idempotent.
const installShellRC = `mkdir -p ` + shellStateDir + ` && cat > ` + shellRC + ` <<'ENTROPY_RC'
` + shellPrologue + `ENTROPY_RC
cat > ` + shellStateDir + `/ptyrc <<'ENTROPY_PTYRC'
` + ptyRC + `ENTROPY_PTYRC
`

func wrapUserCommand(cmd string) string {
	return ". " + shellRC + " 2>/dev/null\n" + cmd + "\n"
}

// interactiveHint returns guidance when a command needs a TTY that the
// sandbox exec channel does not provide. An empty string means run it.
func interactiveHint(cmd string) string {
	fields := strings.Fields(strings.TrimSpace(cmd))
	if len(fields) == 0 {
		return ""
	}
	name := fields[0]
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	switch name {
	case "top", "htop", "atop":
		return name + " needs an interactive screen. Try `top -b -n 1 | head -20` or `ps aux --sort=-%cpu | head`."
	case "vi", "vim", "nano", "emacs", "pico":
		return name + " needs an interactive screen. Write files with `cat > file <<'EOF'` or `printf '...' > file`."
	case "less", "more", "man":
		return name + " needs an interactive screen. Use `cat`, `head`, or `tail` instead."
	case "watch":
		return "watch redraws the screen. Run the command directly, or loop: `for i in 1 2 3; do date; sleep 1; done`."
	case "ssh", "telnet", "ftp":
		return "The sandbox has no network access."
	case "bash", "sh", "python3", "python", "perl", "irb", "node":
		if len(fields) == 1 {
			return name + " without arguments starts an interactive prompt. Pass a script, for example `" + name + " -c '...'`" +
				map[bool]string{true: " (perl uses -e).", false: "."}[name == "perl"]
		}
	}
	return ""
}

// promptPath shortens /root to ~ for the terminal prompt.
func promptPath(cwd string) string {
	cwd = strings.TrimSpace(cwd)
	switch {
	case cwd == "":
		return "~"
	case cwd == "/root":
		return "~"
	case strings.HasPrefix(cwd, "/root/"):
		return "~" + strings.TrimPrefix(cwd, "/root")
	}
	return cwd
}
