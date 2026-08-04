# STATUS

current_stage: P12
stage_state: local_complete / blocked_free_stack
next_stage: P12 free-stack acceptance (operator VM + tunnel)
branch: kalyan_dev
delivery_target: free_vm
architecture: vm_compose

## Summary
Free-stack implementation is in-repo and smoke-tested on local Compose:
Keycloak OIDC, Ollama AI provider, Prometheus/Grafana/Loki, Linux eBPF runbook,
and Cloudflare Tunnel Compose profile. Live public acceptance waits on operator
VM/tunnel inputs in `docs/your-actions.md`.

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
