# P12: Free VM hosting + acceptance

Status: local_complete / blocked_free_stack (operator tunnel + Linux eBPF smoke)
Stage: P12
Requirements: 1-18 (acceptance)

## Goal
Frontend hosted via the free VM (for example Cloudflare Tunnel) and a full PRD
acceptance report against the free target stack.

## Checklist
- [x] docs/acceptance-p12.md with pass/fail per Req 1-18
- [x] Public hosting wiring in repo (cloudflared profile + runbooks)
- [x] Guest auth path locally
- [x] Keycloak on local Compose free stack
- [x] Onboarding, terminal, viz, scenarios
- [x] Snapshot/restore + history
- [x] AI stream + rate limit path via Ollama (model pull may be on-demand)
- [x] Idle/max/disconnect summary locally
- [x] eBPF Linux path documented (`docs/linux-ebpf.md` + compose override)
- [ ] Operator: tunnel HTTPS/WSS live
- [ ] Operator: real eBPF probes on Linux VM
- [ ] STATUS next_stage=none

## Exit criteria
- [x] acceptance-p12.md complete; production blockers listed only if real
- [ ] Live free-stack VM acceptance after `docs/your-actions.md`
