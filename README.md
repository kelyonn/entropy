# Entropy

Entropy is an AI-powered chaos-engineering sandbox. It gives developers a safe,
isolated environment to run dangerous system commands, watch the consequences
through a live terminal and a system-health visualization, and receive AI
root-cause explanations when things fail.

**Primary target:** a free Linux VM running Docker Compose (Keycloak, Ollama,
Prometheus/Grafana/Loki, MinIO, Postgres, Redis), optionally exposed with
Cloudflare Tunnel. See `STATUS.md` and `docs/acceptance-p12.md`.

## Architecture

- Frontend: React + Vite, Xterm.js terminal, React Three Fiber visualization,
  Markdown AI panel, Keycloak OIDC adapter.
- Session Manager: Go HTTP/WebSocket gateway, OIDC JWT validation, lifecycle,
  persistence, Prometheus `/metrics`.
- Airlock: isolated Docker spawn, streaming exec, timeout signaling, statistics,
  and image save/load.
- AI Explainer: Ollama streaming (default), cache, rate limiting, circuit
  breaker, and deterministic fallbacks.
- eBPF Monitor: Linux probes (canonical) with a local simulator using the same
  event shape.
- Data: PostgreSQL, Redis, MinIO (S3-compatible) snapshots.
- Ops: Prometheus, Grafana, Loki, Promtail.

## Labs

The workspace centres on guided labs (`services/session-manager/labs/`):

- **Incidents**: the sandbox is broken on purpose (a CPU hog disguised as a
  kernel thread, a deleted log still holding /var/log, PID exhaustion, zombies,
  permission bugs, a memory leak, a port conflict). Objectives check themselves
  after every command; hints and a debrief are built in.
- **Demos**: OOM kill, fork bomb against the PID limit, CPU throttling, and
  signalling PID 1, each followed by an AI analysis.

Verify every incident against Docker with:

```bash
cd services/session-manager && ENTROPY_LAB_SMOKE=1 go test ./labs/ -run Docker -v
```

## Prerequisites

- Docker and Docker Compose
- Go 1.26+ (for local service development)
- Node.js 20+ (for local frontend dev)
- Linux VM for real eBPF and public hosting (`docs/vm-deploy.md`)

## Run with Docker Compose

```bash
docker compose up --build
```

Then open `http://localhost:3000`.

| Host | Service |
|------|---------|
| `3000` | Frontend (nginx; proxies `/health`, `/api`, `/ws`) |
| `8082` | Session Manager (for Vite proxy) |
| `8087` | Keycloak |
| `3001` | Grafana (`admin` / `admin`) |
| `9090` | Prometheus |
| `3100` | Loki |

On a Linux host with real probes:

```bash
EBPF_SIMULATOR_MODE=false EBPF_PRIVILEGED=true \
  docker compose -f docker-compose.yml -f docker-compose.linux.yml up -d --build
```

See `docs/linux-ebpf.md`.

## Vite + Compose (optional)

```bash
docker compose up --build
cd frontend && npm install && npm run dev
```

Open `http://127.0.0.1:5175`. Vite proxies `/api` and `/ws` to Session Manager
on `127.0.0.1:8082`.

## Verify

```bash
curl -i http://127.0.0.1:3000/
curl -i http://127.0.0.1:3000/health
curl -i http://127.0.0.1:8082/health
curl -i -H 'X-Guest: 1' http://127.0.0.1:3000/api/session/new
curl -i http://127.0.0.1:8082/metrics
```

More local checks: `docs/local-ops.md`.

## Free VM + public URL

Operator steps (VM, tunnel, Keycloak redirects, eBPF, acceptance):
[`docs/your-actions.md`](docs/your-actions.md).

Deploy runbook: [`docs/vm-deploy.md`](docs/vm-deploy.md).
Inputs one at a time: [`docs/production-inputs.md`](docs/production-inputs.md).

## Repository layout

- `services/` — Session Manager, Airlock, AI Explainer, and eBPF Monitor
- `pkg/` — shared service contracts, clients, persistence, auth, resilience
- `frontend/` — React + Vite app
- `docs/` — requirements, design, architecture, UI, ops, decision records
- `memory/` — durable notes for future sessions
- `issues/` — backlog and templates (`PXX` active; `SXX` legacy)
- `infra/` / `k8s/` — legacy AWS packaging (optional; free-stack uses Compose)

## Contributing conventions

- One roadmap stage per change set.
- No emojis anywhere in the product or docs.
- Do not invent additional backends beyond the four services above.
- Prefer the free Compose path over AWS-specific docs when onboarding.
