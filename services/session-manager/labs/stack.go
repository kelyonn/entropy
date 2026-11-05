package labs

import (
	_ "embed"
	"strings"
)

//go:embed stack/stack.py
var stackPy string

//go:embed stack/stackctl
var stackctl string

// stackRequirement fails setup with exit 3 and a clear message when the
// sandbox image lacks python3 (stock ubuntu:22.04).
const stackRequirement = `python3 -c 'import http.server, http.client' >/dev/null 2>&1 || { echo "This lab needs the entropy-sandbox image (python3). Build it with: docker compose build sandbox-image"; exit 3; }
`

// stackSetup installs the production stack, writes configs, and starts it.
// confs overrides a config file (db, app, proxy, loadgen) by name.
func stackSetup(confs map[string]string, extra string) string {
	files := map[string]string{
		"db":      "max_connections=20\nindex_orders=on\n",
		"app":     "listen_port=8000\ndb_port=5432\npool_size=4\nlog_level=info\nhold_connection_ms=0\n",
		"proxy":   "upstream=127.0.0.1:8000\nupstream_timeout_s=2\n",
		"loadgen": "rps=8\n",
	}
	for k, v := range confs {
		files[k] = v
	}
	var b strings.Builder
	b.WriteString(stackRequirement)
	b.WriteString("mkdir -p /opt/stack /etc/stack /var/log/stack /run/stack\n")
	b.WriteString("cat > /opt/stack/stack.py <<'ENTROPY_STACK_PY'\n" + stackPy + "ENTROPY_STACK_PY\n")
	b.WriteString("cat > /usr/local/bin/stackctl <<'ENTROPY_STACKCTL'\n" + stackctl + "ENTROPY_STACKCTL\n")
	b.WriteString("chmod +x /opt/stack/stack.py /usr/local/bin/stackctl\n")
	b.WriteString(": > /var/log/stack/traffic.log\n")
	for _, name := range []string{"db", "app", "proxy", "loadgen"} {
		b.WriteString("cat > /etc/stack/" + name + ".conf <<'ENTROPY_CONF'\n" + files[name] + "ENTROPY_CONF\n")
	}
	b.WriteString(extra)
	b.WriteString("for s in db app proxy loadgen; do stackctl start $s >/dev/null || true; done\nsleep 2\n")
	return b.String()
}

func stackLab(l Lab) Lab {
	l.Stack = true
	l.Category = "Production"
	return l
}

const stackTour = "Start with `stackctl status`, then `curl -si localhost:8080/api/orders`. Logs: `stackctl logs proxy` (or app, db). " +
	"Live traffic: `tail -f /var/log/stack/traffic.log` (columns: time total ok errors p50 p95)."

func init() {
	catalog = append(catalog, productionLabs()...)
	for i := range catalog {
		catalog[i].HintCount = len(catalog[i].Hints)
	}
}

func productionLabs() []Lab {
	sloErr := Objective{ID: "errors", Text: "Error rate below 1 percent over the last 8 seconds", Check: `slo_errors_below 1 8`,
		Regressed: "Errors are back above 1 percent. Check what you changed most recently."}
	sloLat := Objective{ID: "latency", Text: "p95 latency below 150 ms over the last 8 seconds", Check: `slo_p95_below 150 8`,
		Regressed: "p95 latency rose above 150 ms again."}
	return []Lab{
		stackLab(Lab{
			ID: "bad-upstream", Title: "502s after the deploy", Kind: KindIncident, Difficulty: "Intermediate", Minutes: 10,
			Summary: "Every request through the proxy returns 502 Bad Gateway since the last deploy.",
			Briefing: "Status page is red: the API returns 502 for every request since a config deploy ten minutes ago. " +
				"The stack is proxy (8080) -> app (8000) -> db (5432), managed with stackctl. Restore service.",
			Concepts: []string{"reverse proxies", "502 vs 504", "listening ports", "config drift", "SLOs"},
			Objectives: []Objective{sloErr,
				{ID: "services", Text: "All four services are running", Check: `[ "$(stackctl status | grep -c ' running ')" -eq 4 ]`,
					Regressed: "A stack service stopped. `stackctl status` shows which."},
			},
			Hints: []string{
				stackTour,
				"A 502 means the proxy could not reach its upstream. `stackctl logs proxy` names the address it tried.",
				"Compare that address with what is actually listening: `ss -ltnp` or `cat /etc/stack/app.conf`.",
				"Fix `upstream=` in /etc/stack/proxy.conf, then `stackctl restart proxy`.",
			},
			Debrief: "502 Bad Gateway means the proxy is up but its upstream is not answering where it expects; 504 means the upstream " +
				"answered too slowly. The deploy changed the upstream port. Checking what is listening (ss, lsof) against the proxy " +
				"config finds this in a minute; in production, config validation in CI and a canary would have caught it before users did.",
			Setup: stackSetup(map[string]string{"proxy": "upstream=127.0.0.1:8001\nupstream_timeout_s=2\n"}, ""),
		}),
		stackLab(Lab{
			ID: "slow-queries", Title: "Latency tripled overnight", Kind: KindIncident, Difficulty: "Intermediate", Minutes: 12,
			Summary: "The API still answers, but p95 latency went from 10 ms to over 400 ms.",
			Briefing: "No errors, but customers report a sluggish site and the latency SLO is burning. A database maintenance " +
				"job ran overnight. Find where the time goes and bring p95 under 150 ms.",
			Concepts:   []string{"latency percentiles", "following a request through tiers", "slow query logs", "indexes"},
			Objectives: []Objective{sloLat, sloErr},
			Hints: []string{
				stackTour,
				"Time each tier: `curl -s -w '%{time_total}\\n' -o /dev/null localhost:8080/api/orders`, then the same against :8000.",
				"The app waits on the database. `stackctl logs db` shows why each query is slow.",
				"The maintenance job left `index_orders=off` in /etc/stack/db.conf. Turn it back on and `stackctl restart db`.",
			},
			Debrief: "Latency hides in the slowest tier. Timing each hop narrowed it to the database, and its slow-query log named the " +
				"missing index. Full table scans grow with data, so they often look fine in staging and fail in production.",
			Setup: stackSetup(map[string]string{"db": "max_connections=20\nindex_orders=off\n"}, ""),
		}),
		stackLab(Lab{
			ID: "pool-exhaustion", Title: "Too many connections", Kind: KindIncident, Difficulty: "Advanced", Minutes: 12,
			Summary: "Many requests fail with 500 because the app opens more database connections than the database allows.",
			Briefing: "After scaling the app's connection pool for a traffic event, a large share of requests fail. The database " +
				"itself is healthy. Make the app and database agree.",
			Concepts: []string{"connection pools", "database connection limits", "concurrency math", "capacity planning"},
			Objectives: []Objective{sloErr,
				{ID: "fits", Text: "The app's pool_size fits within the database's max_connections",
					Check:     `[ "$(confval app pool_size)" -le "$(confval db max_connections)" ]`,
					Regressed: "The app's pool is larger than the database allows again."},
			},
			Hints: []string{
				stackTour,
				"`stackctl logs app` and `stackctl logs db` show the same failure from both sides.",
				"Compare `pool_size` in /etc/stack/app.conf with `max_connections` in /etc/stack/db.conf.",
				"Do the math: 16 requests per second that each hold a connection for 300 ms need about 5 connections at once. A pool of 3 caps you at 10 requests per second.",
				"Raise max_connections in /etc/stack/db.conf to at least pool_size, then `stackctl restart db`.",
			},
			Debrief: "Each request that holds a database connection counts against max_connections. When the pool is larger than the " +
				"limit, the extra connections are refused under load, and the failures look random. Shrinking the pool below the " +
				"concurrency you need (Little's law: rate times hold time) just trades errors for queueing and timeouts. Size the " +
				"database limit for pool size times app instances, with headroom for admin connections.",
			Setup: stackSetup(map[string]string{
				"db":      "max_connections=3\nindex_orders=on\n",
				"app":     "listen_port=8000\ndb_port=5432\npool_size=12\nlog_level=info\nhold_connection_ms=300\n",
				"loadgen": "rps=16\n",
			}, ""),
		}),
		stackLab(Lab{
			ID: "log-flood", Title: "Debug logging took down the API", Kind: KindIncident, Difficulty: "Intermediate", Minutes: 10,
			Summary: "Someone left debug logging on. /var/log filled up and the API now fails every request.",
			Briefing: "The API started returning 500s a few minutes after an engineer enabled verbose logging to chase a bug. " +
				"Restore service and make sure it does not happen again in an hour.",
			Concepts: []string{"disk full failures", "log levels", "log rotation", "df and du"},
			Objectives: []Objective{sloErr,
				{ID: "disk", Text: "/var/log below 60 percent", Check: `[ "$(usepct /var/log)" -lt 60 ]`,
					Regressed: "/var/log is filling up again."},
				{ID: "level", Text: "The app is not logging at debug level", Check: `! grep -q '^log_level=debug' /etc/stack/app.conf`,
					Regressed: "Debug logging is back on; the disk will fill again."},
			},
			Hints: []string{
				stackTour,
				"`df -h /var/log` and `du -sh /var/log/stack/*` show where the space went.",
				"The rotated debug log is safe to delete. The live one keeps growing while log_level=debug.",
				"Set `log_level=info` in /etc/stack/app.conf, delete /var/log/stack/app-debug.log.1, truncate the live log with `: > /var/log/stack/app.log`, then `stackctl restart app`.",
			},
			Debrief: "Logging is I/O, and I/O fails when the disk is full: here the app's error path itself could not write, so every " +
				"request failed. Freeing space restored service, but only fixing the log level kept it restored. Production systems " +
				"cap log volume with rotation by size and sampling for debug output.",
			Setup: stackSetup(map[string]string{
				"app": "listen_port=8000\ndb_port=5432\npool_size=4\nlog_level=debug\nhold_connection_ms=0\n",
			}, "avail=$(df -k /var/log | awk 'NR==2 {print $4}')\n"+
				"dd if=/dev/zero of=/var/log/stack/app-debug.log.1 bs=1K count=$(( avail - 48 )) 2>/dev/null\n"),
		}),
	}
}
