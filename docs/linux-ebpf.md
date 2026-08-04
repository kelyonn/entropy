# Linux eBPF runtime (canonical PRD path)

macOS and unprivileged local Compose use the eBPF **simulator**. That path is
dev-only. Req 7 acceptance for a real learning deployment requires a **Linux
host** running Compose with real probes attached.

## What “real” means

`services/ebpf-monitor` loads BPF programs from `bpf/entropy.bpf.c` and attaches:

| Probe | Purpose |
|-------|---------|
| tracepoint `syscalls/sys_enter_kill` | Signal / kill-style failures |
| tracepoint `syscalls/sys_enter_exit_group` | Process group exit |
| kprobe `oom_kill_process` | OOM kill |
| kprobe `panic` (best effort) | Kernel panic path |

Events are resolved to a sandbox session via Docker inspect, deduped (5s),
buffered (last 20 per session), and POSTed to Session Manager
`/internal/kernel-events`. Session Manager forwards them to the viz and AI
context over the WebSocket.

## Host requirements

- Linux kernel with BTF / tracepoints available (typical modern Ubuntu LTS or
  similar).
- Docker Engine + Compose.
- Ability to run the `ebpf-monitor` container **privileged** (or equivalent
  caps that allow loading BPF and attaching probes).
- Mounts already declared in `docker-compose.yml`:
  - `/var/run/docker.sock` (session PID → container resolution)
  - `/proc` → `/host/proc:ro`
  - `/sys/fs/bpf`

## Enable real probes

Default Compose keeps the simulator (`EBPF_SIMULATOR_MODE=true`,
`EBPF_PRIVILEGED=false`) so macOS and locked-down hosts still boot.

On the Linux VM:

```bash
# .env (or export)
EBPF_SIMULATOR_MODE=false
EBPF_PRIVILEGED=true

docker compose -f docker-compose.yml -f docker-compose.linux.yml up -d --build ebpf-monitor
```

`docker-compose.linux.yml` forces privileged mode and `SIMULATOR_MODE=false`.

## Smoke checks

1. Health:

```bash
docker compose exec ebpf-monitor wget -qO- http://127.0.0.1:8083/health
# expect: ok
```

2. Logs show probe attach (not simulator-only):

```bash
docker compose logs ebpf-monitor --tail 50
# look for: eBPF probes attached
```

3. Guest session + a crashing command in the UI (or Airlock exec). Confirm a
   kernel / failure event reaches the viz and appears in AI context.

4. Simulator endpoint must be off when real probes are on:

```bash
# should 404 when SIMULATOR_MODE=false
curl -i -X POST http://127.0.0.1:8083/internal/simulate \
  -H 'Content-Type: application/json' \
  -d '{"sessionId":"x","eventType":"oom_kill","pid":1}'
```

(Host port `8083` is not published by default; use `docker compose exec` or
publish temporarily for debugging.)

## Dev-only simulator

When `SIMULATOR_MODE` is not `false` (Compose default), the service emits the
same JSON shape without kernel probes. Use it on macOS and for UI demos. Do not
treat simulator-only runs as Req 7 pass for production acceptance.
