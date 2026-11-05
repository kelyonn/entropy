# Session log

Append-only. Newest entries at the bottom. One block per working session.

## 2026-07-17 — Stage 0: repo context scaffold
- Created durable context: CLAUDE.md, AGENTS.md, ROADMAP.md, STATUS.md,
  README.md.
- Set up `docs/`: copied requirements and design from root, added a
  "Local-first deviations" section to `docs/design.md`, wrote
  `docs/architecture.md` (current vs target plus gap table), `docs/ui.md`
  (Slate Studio system), and ADRs 0001 (local-first) and 0002 (Slate Studio).
- Set up `memory/`: product, architecture, conventions, this log.
- Set up `issues/`: README, templates (bug, feature, spike), and backlog files
  S00 through S13.
- Added GitHub templates: pull request template and issue templates.
- No feature code changed in this stage.
- Next: Stage 1 (stabilize local runtime).

## 2026-07-17 — Stages 1-13 implementation sweep
- S1: Compose healthchecks, .env.example, Dockerfile wget, gitignore.
- S2: Slate Studio redesign across frontend (IBM Plex, light chrome, no emoji).
- S3: session.Manager with statuses, MAX_SESSIONS, idle/max lifetime sweeper.
- S4: executor streaming + 30s timeout; WS command queue capacity 10.
- S5: orchestrator NetworkMode none, CapDrop, pids, tmpfs, no-new-privileges.
- S6: WS envelope protocol; reconnect backoff in useWebSocket; visualization msgs.
- S7: Bedrock InvokeModelWithResponseStream, cache, fallbacks, rate limit 10.
- S8: Onboarding modal + localStorage; Start sandbox creates session.
- S9: Scenario catalog cpu/fork/oom/panic with interrupt.
- S10: Postgres store + MinIO/local snapshot save/restore APIs.
- S11: AuthBar Cognito stub + /api/sessions history.
- S12: eBPF stub remains default; tagged tracer path retained.
- S13: amplify.yml, k8s HPA, docs/monitoring.md, README deploy notes.
- Verified: `go build` and `npm run build` succeed.

## 2026-07-17 — Core demo loop fix pass
- Fixed Docker exec demux (stdcopy) in `backend/container/executor.go`.
- Fixed output chunk sequencing in `useWebSocket.js` / `Terminal.jsx`.
- Fixed crash viz overwrite; stats-only visualization updates preserve status.
- Reset AI state per crash/command; added `ai_question` WS handler.
- Dead container detection after commands; improved panic scenario command.
- Container reuse via `EnsureContainer` in orchestrator.
- Removed synthetic backend terminal prompt on WS connect.
- Added `CommandFlow.jsx` lifecycle strip.
- Teaching-oriented Bedrock prompts + richer fallbacks + "Explain more simply".
- Live CPU/RAM stats ticker (2s) in websocket handler.
- Verified Compose stack healthy; WS smoke test PASS (echo, crash, AI, reset).

## 2026-07-17 — Full PRD delivery P0-P12
- P0: Rewrote STATUS/ROADMAP/architecture; added P00-P12 backlog issues.
- P1: Extracted pkg/ (ws, airlock, ai, ebpf, db, resilience).
- P2: Split services/session-manager, airlock, ai-explainer; Compose multi-service.
- P3: Airlock SIGTERM/SIGKILL timeout, spawn 3x retry, StopGraceful.
- P4: Postgres finalize/commands/kernel/ai; gzip snapshots via Airlock; restore + replay.
- P5: Session queue with wait estimate; session_summary WS + modal.
- P6: Event viz (OOM shake, panic explosion); scenario modal; AI history + code blocks.
- P7: ebpf-monitor service + macOS simulator.
- P8: Cognito JWT middleware; guest path retained.
- P9: Circuit breaker on DB; S3 retries; WS reconnect UX.
- P10: infra/terraform modules (VPC, EKS, RDS, Redis, S3, IAM).
- P11: k8s four-service deploy, HPA, NetworkPolicy, CloudWatch alarms.
- P12: amplify.yml env wiring; docs/acceptance-p12.md.

## 2026-07-17 — PRD verification and production hardening
- Corrected the earlier completion claim: P8, P10, P11, and P12 live AWS exit
  criteria remain blocked because AWS credentials and API TLS inputs are absent.
- Removed the legacy monolith after split-stack WebSocket verification.
- Verified real per-command timeout signaling, spawn retry logs, and graceful
  container survival after exit 124.
- Replaced unsigned MinIO HTTP calls with AWS SigV4 S3, fixed retries, added
  local lifecycle setup, and passed filesystem plus history restore.
- Implemented cancellable disconnect grace, queue admission, idle summaries,
  persisted summary replay, and unit/runtime lifecycle checks.
- Added syntax highlighting, event-specific visualization, scenario
  post-analysis, and the required terminal prompt.
- Added Linux BPF generation and probes, PID-to-session resolution, simulator
  parity, and end-to-end visualization/AI forwarding.
- Replaced the auth prompt stub with Amplify Auth and added Cognito
  RS256/JWKS/issuer/client validation tests.
- Added complete DB and Bedrock circuit breakers, CloudWatch metric export,
  Terraform SNS alarms/log groups, ECR, Secrets Manager, encrypted Multi-AZ
  data services, IRSA, and production deployment scripts.
- Validated all Go modules, the frontend production build, production
  dependency audit, Docker images, Compose demo flow, Terraform, Kubernetes
  manifests, and deployment script syntax.
- Rewrote P12 acceptance with honest PASS/PARTIAL/BLOCKED results.

## 2026-07-20 — Local polish + operator handoff
- Published Session Manager on host `8082` for Vite proxy (avoids host `8080`).
- Proxied `/health` and auth headers through nginx; long WS timeouts.
- Refreshed `.env.example`, CLAUDE/AGENTS port map, conventions, README.
- Added `docs/your-actions.md` as the only remaining human checklist.
- Removed empty legacy `backend/` directories; deploy scripts are executable.
- Docker Desktop was not running for a final Compose smoke in this session.

## 2026-07-20 — Local gap follow-up (docs + wiring)
- Aligned CLAUDE/README/design/architecture/issues with split-stack ports.
- Added `docs/local-ops.md`; linked from README and your-actions.
- Marked P01 done; P08 local_complete; split P12 guest vs Cognito/eBPF; S13 boxes checked.
- Fail-fast Docker client and REDIS_URL parse; dropped unused EBPF_MONITOR_URL.
- Hide Sign in when Cognito is not configured.
- Next: AWS via `docs/your-actions.md` (agent blocked).

## 2026-07-23 — Public close-out blocked on Phase A
- Started public free-stack close-out plan; no VM/.env/tunnel token on this Mac.
- Added `docs/closeout-pasteback.md` and `scripts/vm-bootstrap.sh`.
- STATUS remains `local_complete / blocked_free_stack` until operator paste-back.


## 2026-11-05 — P13 lab engine and workspace
- Replaced the four one-shot scenarios with a lab catalog: 7 break/fix incidents
  (runaway CPU with supervisor, deleted-but-open log, PID exhaustion, zombies,
  permissions, memory leak, port conflict) and 4 demos. Objectives auto-check
  after every command; hints, debriefs, Redis progress, `/api/labs`.
- Verified every incident in Docker under Airlock limits with reference fixes.
- Shell state (cwd, exports) persists via `/root/.entropy/rc`; Ctrl+C now works.
  Found and fixed: background jobs ignored SIGINT, and Airlock's `/bin/sh` kill
  rejected `--`, so timeout kills never reached the process.
- Stats now report PIDs and use a two-sample CPU read (one-shot read was always 0).
- Frontend rebuilt around the lab loop; fixed dropped output chunks from batched state.
- Browser-verified locally (services run natively against Docker); full Compose not run.
