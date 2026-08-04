# S10: Postgres + S3 snapshots

Status: done
Stage: S10

## Goal
Durable history and real snapshot/restore.

## Checklist
- [ ] Postgres schema and Compose service
- [ ] Redis kept for cache/pubsub/rate limit
- [ ] MinIO (local) + docker commit/save path
- [ ] Replace Redis-only restore

## Exit criteria
- [ ] Save/restore works with history replay
- [ ] Local MinIO and AWS S3 docs present
