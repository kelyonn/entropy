package handlers

import (
	"strconv"
	"strings"
	"time"
)

// TypeProcTree carries process tree snapshots while the learner has the
// process view open.
const TypeProcTree = "proctree"

const procTreeMarker = "ENTROPY_PROCTREE"

// procTreeScript lists every process using bash builtins only, so sampling
// still works when the PID limit is nearly exhausted.
const procTreeScript = `# ` + procTreeMarker + `
for d in /proc/[0-9]*; do
  [ "${d#/proc/}" = "$$" ] && continue
  read -r stat < "$d/stat" 2>/dev/null || continue
  read -r _ rss _ < "$d/statm" 2>/dev/null || rss=0
  mapfile -d '' args < "$d/cmdline" 2>/dev/null
  printf '%s\t%s\t%s\n' "$stat" "$rss" "${args[*]}"
done
`

type procInfo struct {
	PID   int     `json:"pid"`
	PPID  int     `json:"ppid"`
	State string  `json:"state"`
	Comm  string  `json:"comm"`
	Args  string  `json:"args"`
	RSSKB int     `json:"rssKb"`
	CPU   float64 `json:"cpu"`
	ticks int
}

// parseProcLine parses "<stat>\t<rss pages>\t<args>". The comm field in
// /proc/<pid>/stat is parenthesized and may contain spaces.
func parseProcLine(line string) (procInfo, bool) {
	parts := strings.SplitN(line, "\t", 3)
	if len(parts) < 2 {
		return procInfo{}, false
	}
	stat := parts[0]
	open := strings.IndexByte(stat, '(')
	close := strings.LastIndexByte(stat, ')')
	if open < 0 || close < open {
		return procInfo{}, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(stat[:open]))
	if err != nil {
		return procInfo{}, false
	}
	rest := strings.Fields(stat[close+1:])
	// rest[0]=state rest[1]=ppid ... rest[11]=utime rest[12]=stime
	if len(rest) < 13 {
		return procInfo{}, false
	}
	ppid, _ := strconv.Atoi(rest[1])
	utime, _ := strconv.Atoi(rest[11])
	stime, _ := strconv.Atoi(rest[12])
	pages, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
	p := procInfo{PID: pid, PPID: ppid, State: rest[0], Comm: stat[open+1 : close], RSSKB: pages * 4, ticks: utime + stime}
	if len(parts) == 3 {
		p.Args = strings.TrimSpace(parts[2])
	}
	if p.Args == "" {
		p.Args = "[" + p.Comm + "]"
	}
	return p, true
}

// hiddenProc reports Entropy's own plumbing: exec wrappers, probes, checks.
func hiddenProc(p procInfo) bool {
	return strings.Contains(p.Args, procTreeMarker) ||
		strings.Contains(p.Args, "entropy-command") ||
		strings.Contains(p.Args, "ENTROPY_CHECK_") ||
		strings.Contains(p.Args, "entropy-signal") ||
		strings.Contains(p.Args, shellStateDir+"/cwd")
}

// buildProcTree parses a sample, hides plumbing (reparenting its children),
// and computes CPU percent from the previous sample's ticks.
func buildProcTree(out string, prev map[int]int, elapsed time.Duration) ([]procInfo, map[int]int) {
	var all []procInfo
	for _, line := range strings.Split(out, "\n") {
		if p, ok := parseProcLine(line); ok {
			all = append(all, p)
		}
	}
	parent := map[int]int{}
	hidden := map[int]bool{}
	for _, p := range all {
		parent[p.PID] = p.PPID
		if hiddenProc(p) {
			hidden[p.PID] = true
		}
	}
	visibleParent := func(pid int) int {
		for i := 0; i < 64 && hidden[pid]; i++ {
			pid = parent[pid]
		}
		return pid
	}
	ticks := make(map[int]int, len(all))
	secs := elapsed.Seconds()
	var procs []procInfo
	for _, p := range all {
		ticks[p.PID] = p.ticks
		if hidden[p.PID] {
			continue
		}
		p.PPID = visibleParent(p.PPID)
		if before, ok := prev[p.PID]; ok && secs > 0 && p.ticks >= before {
			// Linux reports ticks at 100 Hz.
			p.CPU = float64(p.ticks-before) / (100 * secs) * 100
		}
		procs = append(procs, p)
	}
	return procs, ticks
}

// runProcTree samples the process tree while the learner has the view open.
func (c *conn) runProcTree(done <-chan struct{}) {
	ticker := time.NewTicker(1500 * time.Millisecond)
	defer ticker.Stop()
	var prev map[int]int
	var last time.Time
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			if !c.procTreeOn.Load() {
				prev = nil
				continue
			}
			out, _, err := c.execQuiet(c.ctx, procTreeScript)
			if err != nil {
				continue
			}
			now := time.Now()
			procs, ticks := buildProcTree(out, prev, now.Sub(last))
			prev, last = ticks, now
			c.send(TypeProcTree, map[string]interface{}{"procs": procs, "at": now.UnixMilli()})
		}
	}
}
