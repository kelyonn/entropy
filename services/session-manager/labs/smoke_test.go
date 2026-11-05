package labs

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// Reference fixes used only by the Docker smoke test. They prove that every
// incident is solvable inside the sandbox constraints.
var referenceFixes = map[string]string{
	"first-steps-processes": `ps aux
free -m
cat /opt/lesson/.pid > /root/answer.txt
kill "$(cat /opt/lesson/.pid)"`,
	"first-steps-files": `df -h
echo archive.bin > /root/answer.txt
chmod +x /srv/notes/hello.sh
/srv/notes/hello.sh
mkdir -p /root/workspace && echo /root/workspace > /root/.entropy/cwd`,
	"runaway-cpu": `for p in /proc/[0-9]*; do
  [ "${p#/proc/}" = "$$" ] && continue
  c=$(tr '\0' ' ' < $p/cmdline 2>/dev/null)
  case "$c" in *opt/metrics/agent*) kill ${p#/proc/} ;; esac
done
sleep 0.5
for p in /proc/[0-9]*; do
  [ "${p#/proc/}" = "$$" ] && continue
  c=$(tr '\0' ' ' < $p/cmdline 2>/dev/null)
  case "$c" in "[kworker/u4:2]"*) kill ${p#/proc/} ;; esac
done`,
	"deleted-log": `for f in /proc/[0-9]*/fd/*; do
  case "$(readlink $f 2>/dev/null)" in *ledger.log*deleted*) : > "$f" ;; esac
done`,
	"spool-storm": `for p in /proc/[0-9]*; do
  [ "${p#/proc/}" = "$$" ] && continue
  c=$(tr '\0' ' ' < $p/cmdline 2>/dev/null)
  case "$c" in *opt/spool/spoold*) pg=$(cut -d' ' -f5 $p/stat); kill -- -$pg ;; esac
done
sleep 1`,
	"zombie-horde": `for p in /proc/[0-9]*; do
  [ "${p#/proc/}" = "$$" ] && continue
  c=$(tr '\0' ' ' < $p/cmdline 2>/dev/null)
  case "$c" in reaper-svc*) kill ${p#/proc/} ;; esac
done
sleep 1`,
	"locked-config": `chgrp app /srv/app/config.env && chmod 640 /srv/app/config.env && chmod 755 /srv/app/bin/start`,
	"memory-leak": `chmod -x /opt/cache/warmer
for p in /proc/[0-9]*; do
  [ "${p#/proc/}" = "$$" ] && continue
  c=$(tr '\0' ' ' < $p/cmdline 2>/dev/null)
  case "$c" in *opt/cache/warmer*) kill ${p#/proc/} ;; esac
done
sleep 1`,
	"port-conflict": `for p in /proc/[0-9]*; do
  [ "${p#/proc/}" = "$$" ] && continue
  c=$(tr '\0' ' ' < $p/cmdline 2>/dev/null)
  case "$c" in *opt/legacy/httpd*) kill ${p#/proc/} ;; esac
done
sleep 0.5; /opt/web/serve; sleep 0.5`,
}

func dockerExec(t *testing.T, id, script string) (string, int) {
	t.Helper()
	cmd := exec.Command("docker", "exec", id, "/bin/bash", "-c", script)
	out, err := cmd.CombinedOutput()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("docker exec: %v", err)
	}
	return string(out), code
}

// TestIncidentLabsInDocker runs each incident in a container with the same
// limits Airlock applies. Enable with ENTROPY_LAB_SMOKE=1.
func TestIncidentLabsInDocker(t *testing.T) {
	if os.Getenv("ENTROPY_LAB_SMOKE") != "1" {
		t.Skip("set ENTROPY_LAB_SMOKE=1 to run against Docker")
	}
	image := os.Getenv("ENTROPY_LAB_IMAGE")
	if image == "" {
		image = "ubuntu:22.04"
	}
	for _, l := range All() {
		if l.Kind != KindIncident {
			continue
		}
		l := l
		t.Run(l.ID, func(t *testing.T) {
			out, err := exec.Command("docker", "run", "-d", "--rm",
				"--memory", "256m", "--memory-swap", "256m", "--cpus", "0.5", "--pids-limit", "100",
				"--network", "none", "--cap-drop", "ALL",
				"--cap-add", "CHOWN", "--cap-add", "DAC_OVERRIDE", "--cap-add", "SETUID", "--cap-add", "SETGID",
				"--security-opt", "no-new-privileges:true",
				"--tmpfs", "/tmp:rw,noexec,nosuid,size=256m",
				"--tmpfs", "/var/log:rw,noexec,nosuid,size=64m",
				"-t", "-i", image, "/bin/bash").CombinedOutput()
			if err != nil {
				t.Fatalf("docker run: %v %s", err, out)
			}
			id := strings.TrimSpace(string(out))
			defer exec.Command("docker", "rm", "-f", id).Run()

			if _, code := dockerExec(t, id, "mkdir -p /root/.entropy"); code != 0 {
				t.Fatal("state dir")
			}
			if out, code := dockerExec(t, id, l.Setup); code != 0 {
				t.Fatalf("setup exit %d: %s", code, out)
			}
			time.Sleep(3 * time.Second)
			out1, _ := dockerExec(t, id, CheckScript(l))
			before := Evaluate(l, out1, nil)
			if AllMet(l, before) {
				t.Fatalf("objectives already met after setup: %v\n%s", before, out1)
			}
			for id, ok := range before {
				t.Logf("before %s=%v", id, ok)
			}
			if out, code := dockerExec(t, id, referenceFixes[l.ID]); code != 0 {
				t.Logf("fix exit %d: %s", code, out)
			}
			time.Sleep(4 * time.Second)
			out2, _ := dockerExec(t, id, CheckScript(l))
			// Each line of the reference fix counts as a learner command.
			after := Evaluate(l, out2, strings.Split(referenceFixes[l.ID], "\n"))
			if !AllMet(l, after) {
				t.Fatalf("reference fix did not satisfy objectives: %v\n%s", after, out2)
			}
		})
	}
}
