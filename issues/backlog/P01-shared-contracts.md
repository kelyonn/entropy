# P01: Shared contracts

Status: done
Stage: P1
Requirements: 13 (protocol)

## Goal
Extract `pkg/` WS envelope, internal HTTP APIs, DB helpers. Services consume
`pkg/`; the monolith has been removed.

## Checklist
- [x] `pkg/ws/envelope.go` — command, output, visualization, ai_stream, heartbeat, scenario, session_summary
- [x] `pkg/airlock/client.go` — POST /internal/exec (NDJSON), POST /internal/kill
- [x] `pkg/ai/client.go` — POST /internal/explain (SSE)
- [x] `pkg/ebpf/events.go` — kernel event types + ingest payload
- [x] `pkg/db/store.go` — shared Postgres helpers
- [x] Services import `pkg`; `go build` passes

## Exit criteria
- [x] `pkg/` builds
- [x] Demo loop works via Session Manager + Airlock (Compose)
