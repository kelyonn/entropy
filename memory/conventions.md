# Conventions

## Go layout
- `services/session-manager/` — public HTTP/WS, auth, lifecycle, persistence, metrics
- `services/airlock/` — Docker spawn, exec, stats, snapshot image ops
- `services/ai-explainer/` — Bedrock stream, cache, rate limit, fallbacks
- `services/ebpf-monitor/` — Linux probes + simulator
- `pkg/` — shared WS, clients, DB, eBPF types, resilience

## WebSocket envelope
```json
{
  "type": "command|output|visualization|ai_stream|heartbeat|scenario|lab|shell|interrupt|ai_explain|ai_question|session_summary|error|pong",
  "sessionId": "uuid",
  "timestamp": "RFC3339",
  "payload": {}
}
```

Legacy fields (`session_id`, `command`, top-level `data`) are accepted for compatibility.

- `lab` (client): `{action: start|check|hint|abandon, id}`; `scenario` is an alias and legacy ids map to demos.
- `lab` (server): lab view with `state` (preparing|active|completed|abandoned|failed), objectives, revealed hints, debrief.
- `output` completion carries `command`, `durationMs`, and prompt `cwd`.
- `shell` (server): `cwd`, `startedAt`, restored `history`.
- `interrupt` (client): SIGINT to the running command's process group.
- `ai_explain` (client): explain the last failed command again. Every non-zero exit auto-explains (Req 8.1) except Ctrl+C and identical repeats.
- `lab` (server) includes `alerts`: objectives that were met and regressed, with a message.
- `lab` (client) `mode`: practice, interview (no hints/AI, timed, graded), gameday. Action `gameday` starts a game day.
- `gameday` (server): state, index, total, resolved, budget.
- `proctree` (client `{on}`; server `{procs, at}`): process snapshots while the Processes tab is open.
- `/ws/pty?session_id=`: interactive terminal. Binary frames are terminal bytes; text frames
  `{"type":"resize","cols","rows"}` up and `{"type":"exit","code"}` down. Requires `/ws` connected.

## HTTP (Session Manager)
- `GET /api/session/new`
- `GET /api/session/restore?session_id=`
- `POST /api/session/snapshot?session_id=`
- `GET /api/sessions` (JWT or `X-User-Id`; not guest)
- `GET /api/dashboard`
- `GET /api/labs?session_id=` (catalog + progress)
- `POST /api/labs/generate` `{topic, difficulty}` -> job; `GET /api/labs/generate?job=` status
- `GET /api/postmortem?session_id=&lab=`, `GET /api/replay?session_id=|token=`, `POST /api/replay/share?session_id=`
- `GET /health`
- `GET /ws?session_id=` (guest=`1` or `access_token=`)

## Logging
Structured zap JSON in production. No emoji in user-facing strings.

## UI
Slate Studio tokens in `docs/ui.md` and `frontend/src/index.css`.

## Labs
- Catalog: `services/session-manager/labs/catalog.go`. Scripts target stock ubuntu:22.04
  (bash, coreutils, util-linux, perl-base). `/tmp` and `/var/log` are noexec tmpfs.
- Objectives use `Check` (sandbox state) or `Ran` (regex over learner commands since start), never both.
  Add `Regressed` text for any objective a learner can undo.
- Checks match processes with bracket patterns (`'[o]pt/x'`) so the check never matches itself.
- Every new incident needs a reference fix in `labs/smoke_test.go` and must pass
  `ENTROPY_LAB_SMOKE=1 go test ./labs/ -run Docker`.
- Spec format for community and generated labs: `labs/spec.go`. Add community labs as
  `labs/community/<id>.json` and run `go run ./cmd/labctl validate labs/community/<id>.json`.
- Production-stack labs set `Stack: true`, need `entropy-sandbox:latest` (python3), and judge
  SLOs from `/var/log/stack/traffic.log` via `slo_errors_below` / `slo_p95_below`.
- Shell integration: the PTY shell rc (`handlers/shellintegration.go`) emits OSC 633 C/D/A markers.
