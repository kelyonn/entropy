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
  "type": "command|output|visualization|ai_stream|heartbeat|scenario|session_summary|error|pong",
  "sessionId": "uuid",
  "timestamp": "RFC3339",
  "payload": {}
}
```

Legacy fields (`session_id`, `command`, top-level `data`) are accepted for compatibility.

## HTTP (Session Manager)
- `GET /api/session/new`
- `GET /api/session/restore?session_id=`
- `POST /api/session/snapshot?session_id=`
- `GET /api/sessions` (JWT or `X-User-Id`; not guest)
- `GET /api/dashboard`
- `GET /health`
- `GET /ws?session_id=` (guest=`1` or `access_token=`)

## Logging
Structured zap JSON in production. No emoji in user-facing strings.

## UI
Slate Studio tokens in `docs/ui.md` and `frontend/src/index.css`.
