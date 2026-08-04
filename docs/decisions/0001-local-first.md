# ADR 0001: Local-first delivery, Redis before Postgres

Status: accepted
Date: 2026-07-17

## Context
The design document targets a full AWS deployment (Cognito, RDS PostgreSQL, S3,
EKS, Bedrock, CloudWatch). Building all of that up front would block progress on
the core learning experience and make local development slow and expensive. The
existing MVP already uses Redis for session metadata, metrics, and command
history.

## Decision
Build Entropy local-first. Ship a working Docker Compose stack (frontend,
backend, Redis) first and add AWS-dependent capabilities in dedicated later
stages. Keep Redis as the near-term store for session metadata, metrics,
command history, and rate limiting. Introduce PostgreSQL and S3 (via MinIO
locally) in Stage 10, and keep Redis afterward for caching, pub/sub, and rate
limiting.

## Consequences
- Fast local iteration with no AWS account required for core work.
- Snapshot/restore is Redis-metadata replay until Stage 10; it is not real
  container image snapshotting to S3 until then.
- Persistence-dependent requirements (durable history, real snapshots) are
  explicitly deferred and tracked in the roadmap.
- A migration step is required in Stage 10 to move durable data into Postgres
  while retaining Redis for volatile data.

## Superseded locally (2026-07)

Postgres, MinIO/S3-compatible snapshots, and Cognito middleware have landed in
the Compose stack (P4 / P8). Redis remains for lifecycle, metrics, and rate
limiting. Consequences above about Redis-only snapshots are historical; live
AWS RDS/S3/Cognito acceptance is still tracked separately.
