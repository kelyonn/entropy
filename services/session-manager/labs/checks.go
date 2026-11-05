package labs

import (
	"bufio"
	"fmt"
	"strings"
)

// checkPrelude defines small helpers shared by objective checks. Helpers use
// bash builtins where possible so checks stay fast even when the process table
// is nearly full.
const checkPrelude = `
alive() {
  local p args
  for p in /proc/[0-9]*; do
    [ "${p#/proc/}" = "$$" ] && continue
    mapfile -d '' args < "$p/cmdline" 2>/dev/null || continue
    [[ "${args[*]}" =~ $1 ]] && return 0
  done
  return 1
}
proccount() {
  local n=0 p
  for p in /proc/[0-9]*; do n=$((n+1)); done
  echo "$n"
}
zombies() {
  local n=0 p line
  for p in /proc/[0-9]*/stat; do
    read -r line < "$p" 2>/dev/null || continue
    line=${line##*) }
    [ "${line%% *}" = Z ] && n=$((n+1))
  done
  echo "$n"
}
usepct() {
  df -P "$1" | awk 'NR==2 { gsub("%", "", $5); print $5 }'
}
mempct() {
  local cur max
  if [ -r /sys/fs/cgroup/memory.current ]; then
    cur=$(< /sys/fs/cgroup/memory.current); max=$(< /sys/fs/cgroup/memory.max)
  else
    cur=$(< /sys/fs/cgroup/memory/memory.usage_in_bytes); max=$(< /sys/fs/cgroup/memory/memory.limit_in_bytes)
  fi
  case "$max" in max|'') max=268435456 ;; esac
  echo $(( cur * 100 / max ))
}
as_user() {
  local u=$1; shift
  if command -v runuser >/dev/null 2>&1; then
    runuser -u "$u" -- "$@"
  else
    setpriv --reuid="$u" --regid="$u" --init-groups "$@"
  fi
}
`

const (
	markerOK = "ENTROPY_CHECK_OK "
	markerNo = "ENTROPY_CHECK_NO "
)

// CheckScript builds one bash script that evaluates every objective and
// prints a marker line per objective.
func CheckScript(l Lab) string {
	var b strings.Builder
	b.WriteString(checkPrelude)
	for _, o := range l.Objectives {
		fmt.Fprintf(&b, "if ( %s ) >/dev/null 2>&1; then echo '%s%s'; else echo '%s%s'; fi\n",
			o.Check, markerOK, o.ID, markerNo, o.ID)
	}
	return b.String()
}

// ParseCheckOutput maps objective ids to pass/fail. Objectives missing from
// the output (for example when the check could not start) are reported as
// not met.
func ParseCheckOutput(l Lab, out string) map[string]bool {
	result := make(map[string]bool, len(l.Objectives))
	for _, o := range l.Objectives {
		result[o.ID] = false
	}
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(line, markerOK):
			id := strings.TrimPrefix(line, markerOK)
			if _, ok := result[id]; ok {
				result[id] = true
			}
		case strings.HasPrefix(line, markerNo):
			id := strings.TrimPrefix(line, markerNo)
			if _, ok := result[id]; ok {
				result[id] = false
			}
		}
	}
	return result
}

// AllMet reports whether every objective passed.
func AllMet(l Lab, status map[string]bool) bool {
	if len(l.Objectives) == 0 {
		return false
	}
	for _, o := range l.Objectives {
		if !status[o.ID] {
			return false
		}
	}
	return true
}
