// Package labs defines Entropy's guided lab catalog and the scripts that
// prepare, verify, and describe each lab. Labs come in two kinds:
//
//   - incident: the sandbox is deliberately broken, the learner diagnoses and
//     repairs it, and objective checks verify the repair.
//   - demo: a short, observable failure (OOM kill, fork bomb) that runs in the
//     terminal and ends with an AI post-analysis.
//
// Every script targets a stock ubuntu:22.04 userland (bash, coreutils,
// util-linux, perl-base) so labs work even when the richer sandbox image is
// not built. /tmp and /var/log are noexec tmpfs mounts, so executables live
// under /opt and /srv.
package labs

import "strings"

type Kind string

const (
	KindIncident Kind = "incident"
	KindDemo     Kind = "demo"
)

// Objective is one verifiable goal. Check is a bash snippet that exits 0 when
// the objective is met. Checks run with the helper prelude in checks.go.
type Objective struct {
	ID    string `json:"id"`
	Text  string `json:"text"`
	Check string `json:"-"`
}

type Lab struct {
	ID         string      `json:"id"`
	Title      string      `json:"title"`
	Kind       Kind        `json:"kind"`
	Category   string      `json:"category"`
	Difficulty string      `json:"difficulty"`
	Minutes    int         `json:"minutes"`
	Summary    string      `json:"summary"`
	Briefing   string      `json:"briefing"`
	Concepts   []string    `json:"concepts"`
	Objectives []Objective `json:"objectives"`
	Hints      []string    `json:"-"`
	HintCount  int         `json:"hintCount"`
	Debrief    string      `json:"-"`
	// Setup runs silently before an incident starts. Demo runs visibly in the
	// learner's terminal.
	Setup string `json:"-"`
	Demo  string `json:"-"`
}

// AIContext is the short lab description passed to the AI Explainer so its
// answers stay grounded in the active lab without revealing the solution.
func (l Lab) AIContext() string {
	var b strings.Builder
	b.WriteString("The learner is working on the Entropy lab \"")
	b.WriteString(l.Title)
	b.WriteString("\". ")
	b.WriteString(l.Summary)
	b.WriteString(" Goals: ")
	for i, o := range l.Objectives {
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(o.Text)
	}
	if l.Kind == KindDemo {
		b.Reset()
		b.WriteString("The learner just ran the Entropy demo \"")
		b.WriteString(l.Title)
		b.WriteString("\". ")
		b.WriteString(l.Briefing)
		b.WriteString(" Explain what they observed and why the kernel behaved that way.")
		return b.String()
	}
	b.WriteString(". Guide with questions and the next diagnostic step. Do not give the full fix unless asked directly.")
	return b.String()
}

// daemon starts a detached background process that survives the exec that
// launched it and does not hold the exec's output streams open.
func daemon(cmd string) string {
	return "setsid " + cmd + " >/dev/null 2>&1 </dev/null &\n"
}

var catalog = []Lab{
	{
		ID:         "runaway-cpu",
		Title:      "The process that is not a kernel thread",
		Kind:       KindIncident,
		Category:   "Processes",
		Difficulty: "Intermediate",
		Minutes:    10,
		Summary:    "CPU is pinned by a process disguised as a kernel worker, and something keeps restarting it.",
		Briefing: "Pager alert: the sandbox has been at its CPU quota for twenty minutes. " +
			"Nobody deployed anything. Find what is burning CPU, stop it, and make sure it stays stopped.",
		Concepts: []string{"/proc", "process tree", "PPID", "signals", "process masquerading"},
		Objectives: []Objective{
			{ID: "hog", Text: "Stop the process that is consuming the CPU", Check: `! alive '[k]worker/u4:2'`},
			{ID: "supervisor", Text: "Stop whatever keeps restarting it", Check: `! alive '[o]pt/metrics/agent'`},
		},
		Hints: []string{
			"Real kernel threads have an empty /proc/<pid>/cmdline and a parent PID of 2. Compare that with what you find.",
			"Try `ps -eo pid,ppid,pcpu,args --sort=-pcpu | head` or read /proc/<pid>/stat directly if ps is missing.",
			"Killing the busy process is not enough. Look at its PPID and read that parent's cmdline.",
			"Stop the parent first (`kill <ppid>`), then the busy process. Order matters when a supervisor respawns children.",
		},
		Debrief: "Malware and runaway jobs often rename themselves to look like kernel threads. Kernel threads have no " +
			"cmdline and are children of kthreadd (PID 2). A supervisor loop respawned the worker, so the durable fix was " +
			"to stop the parent. In production you would also find what launched the supervisor: a cron entry, a systemd " +
			"unit, or a container entrypoint.",
		Setup: `mkdir -p /opt/metrics
cat > /opt/metrics/agent <<'SCRIPT'
#!/bin/bash
while true; do
  found=0
  for p in /proc/[0-9]*; do
    if tr '\0' ' ' < "$p/cmdline" 2>/dev/null | grep -q '^\[kworker/u4:2\]'; then found=1; fi
  done
  if [ "$found" = 0 ]; then
    perl -e '$0 = "[kworker/u4:2]"; 1 while 1' &
  fi
  sleep 2
done
SCRIPT
chmod +x /opt/metrics/agent
` + daemon("/opt/metrics/agent") + `sleep 1`,
	},
	{
		ID:         "deleted-log",
		Title:      "Disk full, but nothing is there",
		Kind:       KindIncident,
		Category:   "Storage",
		Difficulty: "Intermediate",
		Minutes:    12,
		Summary:    "/var/log is almost full, yet du finds nothing. A running service still holds a deleted file open.",
		Briefing: "/var/log is at 90 percent and climbing. Someone already deleted the big log file, but the space " +
			"never came back. The ledger service must keep running. Reclaim the space without stopping it.",
		Concepts: []string{"df vs du", "inodes and open file descriptors", "/proc/<pid>/fd", "log rotation"},
		Objectives: []Objective{
			{ID: "space", Text: "Bring /var/log usage below 50 percent", Check: `[ "$(usepct /var/log)" -lt 50 ]`},
			{ID: "service", Text: "Keep the ledger service running", Check: `alive '[o]pt/ledger/ledgerd'`},
		},
		Hints: []string{
			"Compare `df -h /var/log` with `du -sh /var/log`. When they disagree, space is held by something du cannot see.",
			"A deleted file is only freed when the last open file descriptor closes. List them with `ls -l /proc/*/fd 2>/dev/null | grep deleted`.",
			"You can truncate an open file through its descriptor without killing the process: `: > /proc/<pid>/fd/<n>`.",
		},
		Debrief: "Unlinking a file removes its name, not its data. The kernel frees the blocks only when no process holds a " +
			"descriptor. That is why df and du disagree. Truncating through /proc/<pid>/fd frees the space while the service " +
			"keeps writing. The durable fix is log rotation that signals the service to reopen its files (copytruncate or " +
			"a reopen signal such as SIGHUP).",
		Setup: `mkdir -p /opt/ledger /var/log/ledger
cat > /opt/ledger/ledgerd <<'SCRIPT'
#!/bin/bash
exec 3>>/var/log/ledger/ledger.log
dd if=/dev/zero bs=1M count=56 >&3 2>/dev/null
rm -f /var/log/ledger/ledger.log
echo "ledger: rotated" > /var/log/ledger/ledger.log.1
while true; do echo "$(date +%s) commit ok" >&3; sleep 5; done
SCRIPT
chmod +x /opt/ledger/ledgerd
` + daemon("/opt/ledger/ledgerd") + `sleep 2`,
	},
	{
		ID:         "spool-storm",
		Title:      "Process table exhaustion",
		Kind:       KindIncident,
		Category:   "Processes",
		Difficulty: "Advanced",
		Minutes:    12,
		Summary:    "A spooler has filled most of the PID quota with workers and refills it when you kill them one by one.",
		Briefing: "New processes intermittently fail with \"Resource temporarily unavailable\". The sandbox has a hard " +
			"limit of 100 processes and most of them are taken. Restore headroom and clear every worker.",
		Concepts: []string{"PID cgroup limit", "process groups", "kill -- -PGID", "orphans and reparenting"},
		Objectives: []Objective{
			{ID: "spawner", Text: "Stop the spooler that creates workers", Check: `! alive '[o]pt/spool/spoold'`},
			{ID: "workers", Text: "Clear every leftover spool-worker", Check: `! alive '^[s]pool-worker'`},
			{ID: "headroom", Text: "Bring the process count below 30", Check: `[ "$(proccount)" -lt 30 ]`},
		},
		Hints: []string{
			"Count processes with `ls -d /proc/[0-9]* | wc -l`. The cgroup limit is 100.",
			"Killing workers one at a time will not work because the spooler replaces them. Find the parent of a spool-worker.",
			"The spooler and its workers share a process group. `ps -eo pid,pgid,args` shows it, or field 5 of /proc/<pid>/stat.",
			"`kill -- -<pgid>` signals every process in a group at once.",
		},
		Debrief: "PID limits protect the host from runaway process creation, but they also make the sandbox fragile: once the " +
			"table is full, even your diagnostic tools cannot start. Killing children while the parent lives just triggers " +
			"respawns. Signalling the whole process group removes parent and children in one step.",
		Setup: `mkdir -p /opt/spool
cat > /opt/spool/spoold <<'SCRIPT'
#!/bin/bash
while true; do
  n=0
  for p in /proc/[0-9]*; do
    if tr '\0' ' ' < "$p/cmdline" 2>/dev/null | grep -q '^spool-worker'; then n=$((n+1)); fi
  done
  while [ "$n" -lt 60 ]; do
    ( exec -a spool-worker sleep 3600 ) &
    n=$((n+1))
  done
  sleep 3
done
SCRIPT
chmod +x /opt/spool/spoold
` + daemon("/opt/spool/spoold") + `sleep 3`,
	},
	{
		ID:         "zombie-horde",
		Title:      "Zombies that will not die",
		Kind:       KindIncident,
		Category:   "Processes",
		Difficulty: "Beginner",
		Minutes:    8,
		Summary:    "Defunct processes are piling up. kill -9 does nothing to them.",
		Briefing: "Monitoring shows a growing number of processes in state Z. The on-call engineer already tried " +
			"kill -9 on each one. Nothing changed. Clear them.",
		Concepts: []string{"process states", "wait()", "zombie reaping", "PID 1 responsibilities"},
		Objectives: []Objective{
			{ID: "zombies", Text: "Leave no zombie processes", Check: `[ "$(zombies)" -eq 0 ]`},
		},
		Hints: []string{
			"Find them with `ps -eo pid,ppid,stat,args | awk '$3 ~ /Z/'` or look for state Z in /proc/<pid>/stat.",
			"A zombie has already exited. There is nothing left to kill. Its entry stays until its parent calls wait().",
			"Look at the PPID of the zombies. If the parent will not reap them, removing the parent hands them to PID 1, which does.",
		},
		Debrief: "A zombie is a finished process whose exit status has not been collected. Signals cannot affect it. The " +
			"parent must call wait(), or exit so the zombie is reparented to PID 1, which reaps it. Containers whose PID 1 " +
			"never reaps (a plain application binary) accumulate zombies, which is why tini or --init exist.",
		Setup: `mkdir -p /opt/reaper
cat > /opt/reaper/reaper-svc <<'SCRIPT'
#!/usr/bin/perl
$0 = "reaper-svc --workers 12";
$SIG{CHLD} = 'DEFAULT';
for (1..12) { my $pid = fork(); exit 0 if defined $pid && $pid == 0; }
sleep 3600 while 1;
SCRIPT
chmod +x /opt/reaper/reaper-svc
` + daemon("/opt/reaper/reaper-svc") + `sleep 1`,
	},
	{
		ID:         "locked-config",
		Title:      "Permission denied on deploy",
		Kind:       KindIncident,
		Category:   "Permissions",
		Difficulty: "Beginner",
		Minutes:    8,
		Summary:    "The app user cannot read its own config or run its start script after a deploy.",
		Briefing: "The deploy finished, but the service fails to start as the unprivileged user app. Root can read " +
			"everything, so it worked when the deployer tested it. Fix it for the app user without making secrets " +
			"world-writable.",
		Concepts: []string{"file ownership", "mode bits", "execute permission", "least privilege", "testing as the service user"},
		Objectives: []Objective{
			{ID: "read", Text: "User app can read /srv/app/config.env", Check: `as_user app test -r /srv/app/config.env`},
			{ID: "start", Text: "User app can run /srv/app/bin/start successfully", Check: `as_user app /srv/app/bin/start >/dev/null 2>&1`},
			{ID: "safe", Text: "The config is not readable or writable by everyone", Check: `[ "$(( 0$(stat -c %a /srv/app/config.env) & 06 ))" -eq 0 ]`},
		},
		Hints: []string{
			"Reproduce the failure as the service user: `runuser -u app -- /srv/app/bin/start`.",
			"`ls -l /srv/app /srv/app/bin` and `id app` show ownership, mode bits, and group membership.",
			"The start script needs the execute bit, and the config needs read access for app through owner or group, not other.",
			"One safe answer: `chgrp app /srv/app/config.env && chmod 640 /srv/app/config.env && chmod 755 /srv/app/bin/start`.",
		},
		Debrief: "Testing as root hides permission bugs because root bypasses most mode checks. Always reproduce as the " +
			"service user. Granting access through the group (640, group app) keeps the secret away from other users, " +
			"while chmod 777 would have fixed the symptom and created a security hole.",
		Setup: `id app >/dev/null 2>&1 || useradd -r -M -s /usr/sbin/nologin app
mkdir -p /srv/app/bin
printf 'DB_URL=postgres://ledger@db/ledger\nAPI_TOKEN=s3cr3t-rotate-me\n' > /srv/app/config.env
chown root:root /srv/app/config.env
chmod 600 /srv/app/config.env
cat > /srv/app/bin/start <<'SCRIPT'
#!/bin/bash
set -e
. /srv/app/config.env
echo "app: config loaded, token length ${#API_TOKEN}"
SCRIPT
chmod 644 /srv/app/bin/start
chmod 755 /srv/app /srv/app/bin`,
	},
	{
		ID:         "memory-leak",
		Title:      "The cache that ate the memory",
		Kind:       KindIncident,
		Category:   "Memory",
		Difficulty: "Intermediate",
		Minutes:    10,
		Summary:    "A cache warmer is holding most of the 256 MB memory limit, and a launcher restarts it.",
		Briefing: "Memory usage is near the cgroup limit and the OOM killer is about to start choosing victims. " +
			"Free the memory and stop the cache warmer from coming back.",
		Concepts: []string{"cgroup memory limit", "RSS", "/proc/<pid>/status", "OOM killer", "respawn loops"},
		Objectives: []Objective{
			{ID: "memory", Text: "Bring memory usage below 40 percent of the limit", Check: `[ "$(mempct)" -lt 40 ]`},
			{ID: "launcher", Text: "Stop the cache warmer from restarting", Check: `! alive '[o]pt/cache/launcher' || [ ! -x /opt/cache/warmer ]`},
		},
		Hints: []string{
			"Read the cgroup: `cat /sys/fs/cgroup/memory.current /sys/fs/cgroup/memory.max` (cgroup v2).",
			"Find the biggest process by resident memory: `grep VmRSS /proc/[0-9]*/status | sort -k2 -n | tail -3`.",
			"If you kill the warmer and memory climbs again, something relaunched it. Check its parent.",
			"Stop the launcher, or remove the execute bit from /opt/cache/warmer so the launcher cannot start it.",
		},
		Debrief: "Container memory limits are enforced per cgroup. When usage reaches memory.max the kernel OOM killer picks " +
			"a victim, which may not be the process that leaked. Identifying the largest RSS and its supervisor lets you fix " +
			"the cause instead of waiting for a random kill.",
		Setup: `mkdir -p /opt/cache
cat > /opt/cache/warmer <<'SCRIPT'
#!/usr/bin/perl
$0 = "/opt/cache/warmer --preload all";
my @blocks;
for (1..15) { push @blocks, "x" x (10*1024*1024); select(undef, undef, undef, 0.3); }
sleep 3600 while 1;
SCRIPT
cat > /opt/cache/launcher <<'SCRIPT'
#!/bin/bash
while true; do
  if [ -x /opt/cache/warmer ]; then /opt/cache/warmer; fi
  sleep 3
done
SCRIPT
chmod +x /opt/cache/warmer /opt/cache/launcher
` + daemon("/opt/cache/launcher") + `sleep 4`,
	},
	{
		ID:         "port-conflict",
		Title:      "Address already in use",
		Kind:       KindIncident,
		Category:   "Networking",
		Difficulty: "Intermediate",
		Minutes:    10,
		Summary:    "The new web service cannot bind port 8080 because a forgotten legacy server still holds it.",
		Briefing: "The release is out, but /opt/web/serve exits with \"Address already in use\". Find what owns port " +
			"8080, retire it, and get the new service listening.",
		Concepts: []string{"listening sockets", "/proc/net/tcp", "socket inodes", "ss and lsof"},
		Objectives: []Objective{
			{ID: "legacy", Text: "Stop the legacy server holding port 8080", Check: `! alive '[o]pt/legacy/httpd'`},
			{ID: "serve", Text: "Start the new service on port 8080", Check: `alive '[w]eb-serve :8080'`},
		},
		Hints: []string{
			"Reproduce it first: `/opt/web/serve`.",
			"`ss -ltnp` or `lsof -i :8080` shows the owner. Without them, port 8080 is 1F90 in hex in /proc/net/tcp.",
			"Match the socket inode from /proc/net/tcp to a `socket:[inode]` link under /proc/*/fd to find the PID.",
			"Stop that process, then run `/opt/web/serve` again.",
		},
		Debrief: "Only one socket can listen on an address and port at a time. Tools like ss and lsof map listening sockets " +
			"to processes by matching socket inodes in /proc. Under deploy pressure, check what holds the port before " +
			"changing the port number.",
		Setup: `mkdir -p /opt/legacy /opt/web
cat > /opt/legacy/httpd <<'SCRIPT'
#!/usr/bin/perl
use IO::Socket::INET;
$0 = "/opt/legacy/httpd --port 8080";
my $s = IO::Socket::INET->new(LocalAddr => "127.0.0.1:8080", Listen => 5, ReuseAddr => 1) or die "bind: $!\n";
while (my $c = $s->accept) { print $c "HTTP/1.0 200 OK\r\n\r\nlegacy\n"; close $c; }
SCRIPT
cat > /opt/web/serve <<'SCRIPT'
#!/usr/bin/perl
use IO::Socket::INET;
use POSIX qw(setsid);
my $s = IO::Socket::INET->new(LocalAddr => "127.0.0.1:8080", Listen => 5, ReuseAddr => 1)
  or die "web-serve: bind 127.0.0.1:8080: $!\n";
print "web-serve: listening on 127.0.0.1:8080\n";
my $pid = fork();
exit 0 if $pid;
setsid();
open STDIN, "<", "/dev/null"; open STDOUT, ">", "/dev/null"; open STDERR, ">", "/dev/null";
$0 = "web-serve :8080";
while (my $c = $s->accept) { print $c "HTTP/1.0 200 OK\r\n\r\nnew\n"; close $c; }
SCRIPT
chmod +x /opt/legacy/httpd /opt/web/serve
` + daemon("/opt/legacy/httpd") + `sleep 1`,
	},
	{
		ID:         "oom-kill",
		Title:      "Watch the OOM killer",
		Kind:       KindDemo,
		Category:   "Memory",
		Difficulty: "Beginner",
		Minutes:    2,
		Summary:    "Allocate past the 256 MB limit and watch the kernel kill the process with exit 137.",
		Briefing:   "This demo allocates memory until the cgroup limit is reached. Watch the memory column climb, then the kill.",
		Concepts:   []string{"cgroup memory.max", "SIGKILL", "exit code 137"},
		Demo: `echo "allocating 8 MB every quarter second inside a 256 MB limit"
perl -e '$| = 1; my @a; for my $i (1..40) { push @a, "x" x (8*1024*1024); print "allocated ", $i*8, " MB\n" if $i % 4 == 0; select(undef, undef, undef, 0.25) }; print "survived\n"'`,
	},
	{
		ID:         "fork-bomb",
		Title:      "Fork bomb against a PID limit",
		Kind:       KindDemo,
		Category:   "Processes",
		Difficulty: "Beginner",
		Minutes:    2,
		Summary:    "Run the classic fork bomb for five seconds and see the PID cgroup contain it.",
		Briefing:   "The bomb recursively spawns itself. The 100-process limit stops it; timeout then kills the whole group.",
		Concepts:   []string{"fork()", "PID cgroup", "process groups"},
		Demo: `timeout -s KILL 5 bash -c ':(){ :|:& };:' 2>/tmp/.forklog
echo "fork failures reported: $(grep -c . /tmp/.forklog)"; rm -f /tmp/.forklog
echo "processes now: $(ls -d /proc/[0-9]* | wc -l)"`,
	},
	{
		ID:         "cpu-spike",
		Title:      "CPU quota throttling",
		Kind:       KindDemo,
		Category:   "Processes",
		Difficulty: "Beginner",
		Minutes:    2,
		Summary:    "Start four busy loops on a half-core quota and watch the scheduler throttle them.",
		Briefing:   "Four workers compete for 0.5 CPU. The telemetry shows the quota ceiling, not four full cores.",
		Concepts:   []string{"CFS quota", "throttling", "cpu.max"},
		Demo: `for i in 1 2 3 4; do timeout 8 perl -e '1 while 1' & done
cat /sys/fs/cgroup/cpu.max 2>/dev/null | awk '{ if ($1 != "max") printf "quota: %.2f CPU\n", $1/$2 }'
wait; echo "throttled for: $(awk '/nr_throttled/ {print $2}' /sys/fs/cgroup/cpu.stat 2>/dev/null) periods"`,
	},
	{
		ID:         "kill-init",
		Title:      "Try to kill PID 1",
		Kind:       KindDemo,
		Category:   "Processes",
		Difficulty: "Beginner",
		Minutes:    2,
		Summary:    "Send SIGKILL to the container init process and see why it survives.",
		Briefing:   "Inside a PID namespace, the kernel drops fatal signals sent to init from within the namespace.",
		Concepts:   []string{"PID namespaces", "init protection", "signal handlers"},
		Demo:       `kill -9 1; echo "kill exit status: $?"; sleep 1; echo "PID 1 is $(tr '\0' ' ' < /proc/1/cmdline)"`,
	},
}

func init() {
	for i := range catalog {
		catalog[i].HintCount = len(catalog[i].Hints)
	}
}

// All returns the catalog in display order.
func All() []Lab {
	out := make([]Lab, len(catalog))
	copy(out, catalog)
	return out
}

// legacyIDs maps the original scenario ids to their lab equivalents.
var legacyIDs = map[string]string{
	"cpu":   "cpu-spike",
	"fork":  "fork-bomb",
	"oom":   "oom-kill",
	"panic": "kill-init",
}

// Get finds a lab by id, accepting legacy scenario ids.
func Get(id string) (Lab, bool) {
	if mapped, ok := legacyIDs[id]; ok {
		id = mapped
	}
	for _, l := range catalog {
		if l.ID == id {
			return l, true
		}
	}
	return Lab{}, false
}

// CleanupScript stops every process a lab may have started so labs do not
// interfere with each other. It skips its own shell.
const CleanupScript = `for p in /proc/[0-9]*; do
  [ "${p#/proc/}" = "$$" ] && continue
  [ "${p#/proc/}" = 1 ] && continue
  mapfile -d '' args < "$p/cmdline" 2>/dev/null || continue
  if [[ "${args[*]}" =~ (opt/(metrics/agent|ledger/ledgerd|spool/spoold|reaper/reaper-svc|cache/launcher|cache/warmer|legacy/httpd)|^spool-worker|^\[kworker/u4:2\]|^reaper-svc|^web-serve) ]]; then
    kill -9 "${p#/proc/}" 2>/dev/null
  fi
done
true`
