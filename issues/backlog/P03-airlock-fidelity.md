# P03: Airlock PRD fidelity

Status: complete
Stage: P3
Requirements: 4, 6, 14, 18.1

## Goal
SIGTERM/SIGKILL on timeout; spawn retry; graceful container stop.

## Checklist
- [x] 30s timeout sends SIGTERM, wait 5s, SIGKILL
- [x] Container create retries 3x exponential backoff
- [x] Stop: SIGTERM + 5s grace then force remove
- [x] Command queue capacity 10 with clear error
- [x] `sleep 35` exits 124 with process killed

## Exit criteria
- [x] Timeout kills exec; spawn retries visible in logs
