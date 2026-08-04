# P12 acceptance report (free stack)

Date: 2026-07-22 (updated)

Local environment: Docker Compose with PostgreSQL, Redis, MinIO, Keycloak,
Ollama, Session Manager, Airlock, AI Explainer, eBPF (simulator on macOS;
Linux probes via `docker-compose.linux.yml`), Prometheus, Grafana, Loki,
Promtail, and frontend.

Free-stack public environment: operator-dependent. Repo wiring for Cloudflare
Tunnel (`cloudflared` Compose profile) and Linux eBPF is in place; live tunnel
hostname and real probe attach on a remote VM are not claimed as passed here.

## Requirement results

1. Authentication and access — PASS locally. Guest access works. Keycloak realm
   import, frontend `keycloak-js` adapter, and Session Manager OIDC/JWKS
   validation are wired. Public redirect URI / issuer alignment still needs the
   operator tunnel hostnames.
2. Welcome and onboarding — PASS locally. First-use onboarding includes the
   required dangerous examples and the isolation notice.
3. Session creation and orchestration — PASS locally for UUID creation,
   resource limits, WebSocket setup, queue admission, and the required
   `root@entropy:~#` prompt. The three-second production SLA is not load-tested.
4. Interactive terminal — PASS locally for Xterm input, history, streaming
   stdout/stderr, and non-zero exit display.
5. 3D visualization — PARTIAL. Healthy, non-zero, OOM, panic, and segfault
   states are implemented. A formal 60 FPS benchmark has not been run.
6. Command execution and isolation — PASS locally. Timeout verification showed
   exit 124 after SIGTERM/SIGKILL while the sandbox container remained alive.
   Docker configuration disables networking, drops capabilities, and enforces
   CPU, memory, and PID limits.
7. eBPF observability — PARTIAL. Simulator events drive persistence, viz, and
   AI end to end. Linux BPF object builds and probe attach code paths exist;
   `docs/linux-ebpf.md` + `docker-compose.linux.yml` define the canonical
   runtime. Real probe attach is not claimed on this macOS host.
8. AI root-cause explanation — PASS locally for streaming contract, Markdown,
   teaching prompts, eBPF context, history, rate limiting, fallback, and Ollama
   provider wiring. First-token quality depends on model pull/runtime; fallback
   remains if Ollama is cold or unavailable.
9. Guided scenarios — PASS locally. Pre-context, interruption, resource
   overlays, and post-execution analysis are implemented.
10. Snapshot and restore — PARTIAL. MinIO verification passed for gzip image
    save/load, filesystem restoration, PostgreSQL metadata, and history replay.
    The two-second fetch and three-second full-restore SLAs are not benchmarked.
11. Session lifecycle — PASS locally for queueing, five-second disconnect
    detection, reconnect grace, idle termination, hard lifetime, graceful stop,
    persisted summaries, and summary UI. Prometheus/Loki receive lifecycle
    signals locally.
12. Persistence — PASS locally. PostgreSQL writes, pooling, MinIO/S3-compatible
    storage, lifecycle policy, and restore pass on Compose. Managed-service
    failover is N/A on the free single-VM target.
13. WebSocket protocol — PASS locally. Typed envelopes, streaming output,
    visualization, AI tokens, heartbeat, one-megabyte support, and bounded
    exponential reconnect are implemented.
14. Security and isolation — PARTIAL. Local Docker controls, OIDC JWT checks,
    and guest separation are implemented. No production penetration test has
    run on a public tunnel endpoint.
15. Performance and scalability — PARTIAL. Compose targets concurrent sessions
    via `MAX_SESSIONS`; no 100-session VM load test or latency percentile test
    has run.
16. Monitoring and observability — PASS locally. Session Manager `/metrics` is
    scraped by Prometheus; Grafana datasources/dashboard provision; Loki
    ingests labeled container logs; alert rules `EntropyHighErrorRate` and
    `EntropyHighAILatency` load and can fire.
17. Frontend UI and hosting — PARTIAL. Production frontend builds and serves on
    Compose nginx. Public HTTPS/WSS via Cloudflare Tunnel is runbooked
    (`docs/vm-deploy.md`) but not live-validated without operator token/DNS.
18. Error handling and resilience — PASS locally for LLM fallback, PostgreSQL
    circuit opening, S3-compatible snapshot retry behavior, spawn retry, and
    WebSocket reconnect UX.

## Verified commands / signals (local)

- Guest session create via `http://127.0.0.1:3000/api/session/new`
- Session Manager `/health` and `/metrics` (Prometheus text)
- Prometheus query `entropy_total_commands`
- Loki range query `{service="session-manager"}`
- Prometheus rules include Entropy alert group
- Compose services: Keycloak, Ollama, Prometheus, Grafana, Loki, Promtail

## Remaining operator blockers

- Cloudflare Tunnel token + public hostnames (`docs/your-actions.md` items 5–6)
- Keycloak redirect URIs / `VITE_OIDC_*` for those hostnames
- Linux VM eBPF smoke with `docker-compose.linux.yml` (`docs/linux-ebpf.md`)
- Optional: VM load test and public penetration pass
