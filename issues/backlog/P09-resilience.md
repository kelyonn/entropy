# P09: Resilience

Status: complete
Stage: P9
Requirements: 18

## Goal
Circuit breakers; S3 retries; reconnect UX polish.

## Checklist
- [x] Circuit breaker for Postgres and Bedrock
- [x] S3 upload 2 retries
- [x] Bedrock failure banner + fallback
- [x] WS CONNECTING state; disable terminal input while reconnecting
- [x] Container spawn retry (if not in P3)

## Exit criteria
- [x] Kill Bedrock creds: usable fallback; kill DB: circuit opens
