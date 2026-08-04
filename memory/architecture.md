# Architecture memory (living)

Keep this current as the system evolves. For the full target design see
`docs/design.md`; for the gap table see `docs/architecture.md`.

## Runtime shape (current)
- Session Manager: public HTTP/WebSocket gateway, lifecycle, persistence,
  Prometheus `/metrics`, and orchestration clients.
- Airlock: internal Docker Engine API boundary for sandbox spawn, streaming
  exec, timeout signaling, statistics, snapshots, and graceful stop.
- AI Explainer: Ollama streaming (default) with rate limits, cache, circuit
  breaker, and teaching fallbacks.
- eBPF Monitor: Linux BPF probes (canonical) and the same event contract via
  the macOS/local simulator.
- React + Vite frontend: terminal, visualization, AI panel, dashboard,
  Keycloak OIDC adapter.
- PostgreSQL: sessions, commands, kernel events, snapshots, and explanations.
- Redis: active lifecycle state, queues, metrics, command replay, AI cache, and
  rate limiting.
- MinIO (S3-compatible): signed gzip snapshot storage with 30-day lifecycle.
- Keycloak: OIDC realm/client for signed-in learners; guest remains available.
- Observability: Prometheus, Grafana, Loki, Promtail.
- Docker: one constrained Ubuntu container per session, owned by Airlock.

## Service modules
- `services/session-manager`: public routes, OIDC JWT verification (`pkg/auth`),
  session queue/sweeper, WebSocket protocol, restore, PostgreSQL, MinIO, metrics.
- `services/airlock`: Docker-only internal APIs and command execution.
- `services/ai-explainer`: Ollama SSE and deterministic fallbacks.
- `services/ebpf-monitor`: generated BPF object, Linux attachments, resolver,
  deduplication, circular buffers, and simulator.
- `pkg`: shared WS, internal HTTP, persistence, auth, eBPF, and resilience.

## Frontend modules
- `App.jsx`: shell and session bootstrap.
- `components/Terminal.jsx`: Xterm terminal, key handling, output rendering.
- `components/ServerViz.jsx`: R3F health cube with animation.
- `components/AIPanel.jsx`: Markdown explanation drawer, restore button.
- `components/Dashboard.jsx`: metric tiles.
- `hooks/useWebSocket.js`: connect, reconnect, heartbeat, message handling.
- `auth.js`: Keycloak check-sso / login / logout.

## WebSocket messages
Typed envelope: `{ type, sessionId, timestamp, payload }`.
Client types include `command`, `heartbeat`, `scenario`, and `ai_question`.
Server types include `output`, `visualization`, `crash`, `ai_stream`,
`scenario`, `session_summary`, `error`, and `pong`.
Legacy command fields remain accepted for compatibility.

## Production shape (free stack)
Single Linux VM runs Docker Compose for all services above. Cloudflare Tunnel
(`cloudflared` Compose profile) exposes HTTPS/WSS. Linux eBPF uses
`docker-compose.linux.yml`. Operator steps: `docs/your-actions.md`,
`docs/vm-deploy.md`. Legacy AWS Terraform/k8s/Amplify remain in-repo but are
not the primary delivery path.
