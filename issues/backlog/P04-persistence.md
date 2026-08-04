# P04: Persistence completeness

Status: complete
Stage: P4
Requirements: 10, 12

## Goal
Full Postgres writes; gzip S3 snapshots; restore from loaded image + history replay.

## Checklist
- [x] Insert commands, kernel_events, ai_explanations
- [x] Finalize terminated_at, commands_executed, crashes_encountered
- [x] Snapshot: commit, gzip tar, S3 put with 2 retries
- [x] Restore: docker load, spawn from image, WS history replay
- [x] S3 lifecycle 30d (infra or MinIO script)

## Exit criteria
- [x] Save/restore filesystem + terminal history on MinIO
