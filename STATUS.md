# STATUS

current_stage: P14
stage_state: local_complete (P12 still blocked_free_stack on operator inputs; gVisor validation pending on the VM)
next_stage: P12 free-stack acceptance (operator VM + tunnel)
branch: kalyan_dev
delivery_target: free_vm
architecture: vm_compose

## Summary
Free-stack implementation is in-repo and smoke-tested on local Compose:
Keycloak OIDC, Ollama AI provider, Prometheus/Grafana/Loki, Linux eBPF runbook,
and Cloudflare Tunnel Compose profile. Live public acceptance waits on operator
VM/tunnel inputs in `docs/your-actions.md`.

## P14 platform depth (verified locally 2026-11-05)
- Isolation: gVisor runtime selection (`SANDBOX_RUNTIME`), minimal capabilities,
  WebSocket Origin allowlist (`ALLOWED_ORIGINS`). See `docs/isolation.md`.
- Interactive PTY terminal with shell integration (vim/top/job control work).
- 15 incidents: 2 starters, 7 system, 4 production-stack (SLO-judged), 1 community,
  plus AI-generated labs that must pass sandbox validation before publishing.
- Postmortems, shareable replays, live process tree, interview mode, game day, skills.
- CI (`.github/workflows/ci.yml`): Go vet/race, frontend build, lab Docker validation, Playwright.
- Before public launch: install gVisor on the VM and set `SANDBOX_RUNTIME=runsc`.

## P13 lab engine (verified locally 2026-11-05)
- 7 incident labs (processes, storage, memory, permissions, networking) and 4 demos;
  each incident is verified solvable in Docker under Airlock limits
- Objectives auto-check after every command; hints, debriefs, saved progress
- Shell: `cd`/`export` persist, Ctrl+C interrupts the process group, typeahead,
  multi-line paste, interactive-program guidance
- Telemetry: CPU, memory, and PID count with sparklines and a 3D gauge rack
- Compose builds `entropy-sandbox:latest` (`Dockerfile.sandbox`); Airlock falls back to ubuntu:22.04

## What works (verified locally 2026-07-22)
- `docker compose up --build` — UI `http://localhost:3000`, SM `http://127.0.0.1:8082`
- Keycloak `http://127.0.0.1:8087`, Grafana `3001`, Prometheus `9090`, Loki `3100`
- `/health` ok; guest `/api/session/new`; `/metrics` scraped
- Loki ingest for `service=session-manager`; Prometheus Entropy alerts load
- Docs: `docs/vm-deploy.md`, `docs/linux-ebpf.md`, updated monitoring/ops/README

## Next action
**You (production/operator):** paste Phase A inputs from
`docs/closeout-pasteback.md` (Linux VM access, Cloudflare tunnel token set,
`app`/`auth` hostnames). Then continue `docs/your-actions.md` / run
`scripts/vm-bootstrap.sh` on the VM.

**You (local only):** nothing else required; see `docs/local-ops.md`.

## Blockers (production only)
- Waiting on operator Phase A paste-back (`docs/closeout-pasteback.md`)
- Live Cloudflare Tunnel HTTPS/WSS not yet operator-validated
- Real eBPF probes not yet validated on the operator Linux VM
- Public Keycloak redirect/issuer alignment pending tunnel hostnames
