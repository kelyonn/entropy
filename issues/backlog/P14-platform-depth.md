# P14: Platform depth (isolation, real terminal, production incidents, modes)

Status: local_complete (gVisor run on the VM pending)
Stage: P14

## Goal
Make Entropy safe to expose and broad enough to train SREs: real isolation, a
real terminal, multi-tier production incidents, recorded postmortems, a live
process view, assessment modes, and validated generated labs.

## Checklist
- [x] Isolation: SANDBOX_RUNTIME (auto prefers gVisor, named runtime fails closed);
      caps reduced to CHOWN, SETUID, SETGID; ulimits; WebSocket Origin allowlist
- [ ] Run `ENTROPY_LAB_RUNTIME=runsc` lab validation on the VM (gVisor unavailable here)
- [x] Interactive PTY terminal (Airlock /internal/pty, Session Manager /ws/pty) with
      OSC 633 shell integration keeping exit codes, explanations, objectives, timeline
- [x] CI: go vet + race tests, frontend build, lab Docker validation, Playwright e2e
- [x] Production stack (proxy, app, db, load generator) and 4 SLO-judged incidents
- [x] Postmortems (deterministic Markdown) and shareable replays
- [x] Live process tree (Processes tab)
- [x] Interview mode (graded), game day (error budget), skill path
- [x] Lab spec format, community labs, ValidateInSandbox, labctl, AI generator
      that publishes only validated labs

## Exit criteria
- [x] All 14 incidents pass Docker validation on entropy-sandbox:latest
- [x] Browser runs: PTY top/Ctrl+C, stack incident resolved, postmortem + replay,
      process tree, interview graded, game day running, generated lab published
- [x] GitHub Actions green on the branch
