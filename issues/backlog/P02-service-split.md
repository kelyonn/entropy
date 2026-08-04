# P02: Service split

Status: complete
Stage: P2
Requirements: (architecture)

## Goal
Session Manager, Airlock, AI Explainer as separate services; Compose multi-service.

## Checklist
- [x] `services/session-manager/` — HTTP, WS, session, restore
- [x] `services/airlock/` — spawn, exec, stats
- [x] `services/ai-explainer/` — Bedrock, rate limit
- [x] docker-compose.yml wires three services + healthchecks
- [x] Frontend proxies only to session-manager
- [x] Remove or shim `backend/` after cutover
- [x] WS smoke test PASS

## Exit criteria
- [x] `docker compose up` demo loop works through split
