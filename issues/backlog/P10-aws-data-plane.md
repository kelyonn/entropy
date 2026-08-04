# P10: Free VM data plane

Status: local_complete / blocked_free_stack (operator VM boot)
Stage: P10
Requirements: 12, 14, 15 (infra)

## Goal
Docker Compose on a free Linux VM: Postgres, Redis, MinIO, plus the PRD services
and the remaining free dependencies (Keycloak, Ollama, OSS observability).

## Checklist
- [x] Local Compose stack boots (Session Manager, Airlock, AI Explainer, eBPF, Postgres, Redis, MinIO)
- [x] Compose includes Keycloak + Ollama
- [x] OSS observability stack (Prometheus/Grafana/Loki) is reachable locally
- [x] Snapshot persistence and restore verified on local Compose MinIO
- [x] Public exposure path documented (`cloudflared` profile + `docs/vm-deploy.md`)
- [ ] Operator: `docker compose up` on target VM + tunnel hostname smoke

## Exit criteria
- [x] Compose production wiring documented and runnable locally
- [ ] Operator: smoke connect on the target VM (UI/API/WS reachable)
