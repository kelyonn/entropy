# Entropy Roadmap

Two tracks exist in this repo:

1. **S0-S13 (done):** Local-first MVP — Compose, monolith backend, demo loop.
2. **P0-P12 (active):** Full PRD delivery — free-stack production + service split.

Work one P-stage per change set. Do not start the next until exit criteria pass. Detail lives in `issues/backlog/PXX-*.md`.

---

## Free PRD delivery track (P0-P12)

### P0 — Correct status and PRD gap backlog
Rewrite STATUS, ROADMAP, architecture; add P00-P12 backlog files mapped to Req 1-18.

Exit: STATUS current=P0, next=P1; every requirement ID in exactly one backlog file.

### P1 — Extract shared contracts
`pkg/` WS envelope, internal Airlock/AI/eBPF APIs, DB helpers. Monolith unchanged.

Exit: `pkg/` builds; monolith still runs.

### P2 — Split Session Manager / Airlock / AI Explainer
Three services + Compose; frontend talks only to Session Manager.

Exit: `docker compose up` demo loop PASS through split.

### P3 — Airlock PRD fidelity (Req 6, 4, 14)
SIGTERM then SIGKILL on timeout; spawn retry 3x; graceful stop.

Exit: `sleep 35` kills process; spawn retries in logs.

### P4 — Persistence completeness (Req 10, 12)
Full Postgres writes; gzip S3 snapshots; restore from loaded image + history replay.

Exit: Save/restore with filesystem + terminal history on MinIO.

### P5 — Session lifecycle UX (Req 3, 11)
Queue with wait estimate; `session_summary` modal; disconnect timing.

Exit: Summary UI on terminate; queue when at cap.

### P6 — Visualization + scenarios + AI (Req 5, 8, 9, 17)
Event-specific animations; scenario pre-context; AI history + syntax highlight.

Exit: Panic vs OOM viz; scenario modal; AI conversation list.

### P7 — eBPF Monitor (Req 7, 12)
Linux DaemonSet + macOS simulator; wire to viz/AI.

Exit: Linux enriches AI; macOS simulator drives demo.

### P8 — Keycloak auth (Req 1, 11)
OIDC/JWT validation + auth adapter; guest retained.

Exit: Real Keycloak sign-in + guest; history JWT-gated on the free target stack.

### P9 — Resilience (Req 18)
Circuit breakers; S3 retries; reconnect UX.

Exit: Bedrock/DB failure degrades gracefully.

### P10 — Free VM data plane
Compose + VM runtime (Postgres, Redis, MinIO) plus free observability components.

Exit: Compose stack boots on the target VM; smoke connect documented.

### P11 — VM deploy + OSS observability (Req 15, 16)
Prometheus metrics, Grafana dashboards, Loki logs, and alerting rules.

Exit: VM dashboards and alerts validate on real requests.

### P12 — Public hosting + free-stack acceptance
Frontend hosted via the free VM + tunnel; `docs/acceptance-p12.md` Req 1-18 pass/fail.

Exit: Acceptance report complete; STATUS next_stage=none.

---

## Legacy track (S0-S13, complete)

S0 context scaffold through S13 cloud packaging stubs. See `issues/backlog/S00-context.md` through `S13-cloud.md`. These stages built the MVP; P-stages close PRD gaps for production.
