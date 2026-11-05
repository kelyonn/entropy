package labs

import (
	"context"
	"os"
	"testing"
	"time"
)

// Reference fixes used only by the Docker smoke test. They prove that every
// incident is solvable inside the sandbox constraints.
var referenceFixes = map[string]string{
	"bad-upstream":    "sed -i 's/8001/8000/' /etc/stack/proxy.conf && stackctl restart proxy && sleep 12",
	"slow-queries":    "sed -i 's/index_orders=off/index_orders=on/' /etc/stack/db.conf && stackctl restart db && sleep 12",
	"pool-exhaustion": "sed -i 's/^max_connections=.*/max_connections=20/' /etc/stack/db.conf && stackctl restart db && sleep 12",
	"log-flood": `sed -i 's/^log_level=.*/log_level=info/' /etc/stack/app.conf
: > /var/log/stack/app-debug.log.1
: > /var/log/stack/app.log
stackctl restart app && sleep 12`,
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

// TestIncidentLabsInDocker validates every incident (built-in and
// community) in a container with Airlock's limits: broken after setup, fixed
// by the reference fix. Enable with ENTROPY_LAB_SMOKE=1.
func TestIncidentLabsInDocker(t *testing.T) {
	if os.Getenv("ENTROPY_LAB_SMOKE") != "1" {
		t.Skip("set ENTROPY_LAB_SMOKE=1 to run against Docker")
	}
	runtime := os.Getenv("ENTROPY_LAB_RUNTIME") // for example runsc
	image := os.Getenv("ENTROPY_LAB_IMAGE")
	if image == "" {
		image = "ubuntu:22.04"
	}
	factory := NewDockerRunnerFactory(image, runtime)
	community := map[string]Spec{}
	if specs, err := CommunitySpecs(); err == nil {
		for _, s := range specs {
			community[s.ID] = s
		}
	}
	for _, l := range All() {
		if l.Kind != KindIncident {
			continue
		}
		if l.Stack && image == "ubuntu:22.04" {
			t.Logf("skipping %s: needs ENTROPY_LAB_IMAGE=entropy-sandbox:latest", l.ID)
			continue
		}
		l := l
		fix, settle := referenceFixes[l.ID], 4*time.Second
		if spec, ok := community[l.ID]; ok {
			fix = spec.ReferenceFix
			if spec.SettleSeconds > 0 {
				settle = time.Duration(spec.SettleSeconds) * time.Second
			}
		}
		t.Run(l.ID, func(t *testing.T) {
			if fix == "" {
				t.Fatalf("no reference fix for %s", l.ID)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			r := ValidateInSandbox(ctx, factory, l, fix, settle)
			if !r.OK {
				t.Fatalf("validation failed: %v\nafter setup: %v\nafter fix: %v\nsetup output: %s", r.Problems, r.AfterSetup, r.AfterFix, r.SetupOutput)
			}
		})
	}
}
