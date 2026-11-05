# P13: Lab engine and learner workspace

Status: local_complete
Stage: P13
Requirements: 2, 4, 5, 8, 9, 17 (depth pass on the core learning loop)

## Goal
Turn the four one-shot scenarios into a real practice loop: break/fix incident
labs with automatic objective checks, hints, debriefs, and saved progress; a
terminal that behaves like a shell; live telemetry that shows limits being hit.

## Checklist
- [x] `services/session-manager/labs`: catalog of 7 incidents + 4 demos, check
      prelude, progress store (Redis, best run per learner)
- [x] Incidents verified in Docker under Airlock limits
      (`ENTROPY_LAB_SMOKE=1 go test ./labs/ -run Docker`)
- [x] WS `lab` actions: start, check, hint, abandon; auto-check after each command
- [x] `GET /api/labs?session_id=` catalog + progress
- [x] Shell state: cwd and exported variables persist (`/root/.entropy/rc`)
- [x] Ctrl+C: WS `interrupt` -> Airlock `/internal/kill` (process group SIGINT, TERM fallback)
- [x] Airlock: job-control process groups, bash control scripts, PID + memory bytes in stats,
      CPU from a two-sample read, optional `SANDBOX_IMAGE` with ubuntu fallback
- [x] Crash vs ordinary failure: only signals/timeouts auto-explain; others on request
- [x] AI requests carry lab context (`ExplainRequest.Context`)
- [x] Frontend: line editor (cursor, history, kill-line, paste, typeahead), lab panel,
      mission view, telemetry gauges + sparklines, activity timeline, AI subject/history
- [x] Session summary includes labs resolved

## Exit criteria
- [x] Go tests pass in pkg and all services; frontend production build passes
- [x] Browser run: lab start -> objectives tick -> resolved with debrief; OOM demo kills at 137
