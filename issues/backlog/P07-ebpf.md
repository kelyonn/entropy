# P07: eBPF Monitor

Status: complete (simulator); Linux probes are the PRD verification path
Stage: P7
Requirements: 7, 12

## Goal
Linux eBPF service + macOS simulator; wire to viz and AI.

## Checklist
- [x] `services/ebpf-monitor/` with BPF build on Linux
- [x] Trace sys_enter_kill, sys_exit_group, oom_kill_process
- [x] 5s dedup, last-20 buffer per session
- [x] POST /internal/kernel-events to Session Manager
- [x] macOS simulator emits same JSON shape
- [x] SM forwards events to viz + AI context
- [x] Linux Compose override + runbook (`docker-compose.linux.yml`, `docs/linux-ebpf.md`)
- [x] `/health` reports `mode=simulator|probes`

## Exit criteria
- [x] Linux enriches AI; macOS simulator drives demo; app starts without eBPF
- [ ] Operator: real probes validated on free Linux VM (`docs/linux-ebpf.md` smoke)
