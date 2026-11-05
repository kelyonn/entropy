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
- `ai_explain` (client): explain the last failed command. Only crash-class exits (signals, 124, 99) auto-explain.

## HTTP (Session Manager)
- `GET /api/session/new`
- `GET /api/session/restore?session_id=`
- `POST /api/session/snapshot?session_id=`
- `GET /api/sessions` (JWT or `X-User-Id`; not guest)
- `GET /api/dashboard`
- `GET /api/labs?session_id=` (catalog + progress)
- `GET /health`
- `GET /ws?session_id=` (guest=`1` or `access_token=`)

## Logging
Structured zap JSON in production. No emoji in user-facing strings.

## UI
Slate Studio tokens in `docs/ui.md` and `frontend/src/index.css`.

## Labs
- Catalog: `services/session-manager/labs/catalog.go`. Scripts target stock ubuntu:22.04
  (bash, coreutils, util-linux, perl-base). `/tmp` and `/var/log` are noexec tmpfs.
- Checks match processes with bracket patterns (`'[o]pt/x'`) so the check never matches itself.
- Every new incident needs a reference fix in `labs/smoke_test.go` and must pass
  `ENTROPY_LAB_SMOKE=1 go test ./labs/ -run Docker`.
