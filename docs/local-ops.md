# Local ops checklist

Supported path: Docker Compose. Optional: Vite against Session Manager on host
`8082`. Free-VM / tunnel steps: `docs/your-actions.md` and `docs/vm-deploy.md`.

## Port map

| Host | Role |
|------|------|
| `3000` | Frontend nginx (`/`, `/health`, `/api`, `/ws`) |
| `8082` | Session Manager (Compose publish → container `8080`) |
| `8087` | Keycloak |
| `3001` | Grafana |
| `9090` | Prometheus |
| `3100` | Loki |
| `5175` | Vite (optional; proxies to `8082`) |

Compose-internal only: Airlock `8081`, AI Explainer `8082`, eBPF `8083`, Redis,
Postgres, MinIO, Ollama, Promtail. Host `8082` is Session Manager, not AI
Explainer.

Compose hardcodes `DATABASE_URL`, Redis, and MinIO URLs in `docker-compose.yml`.
`.env` overrides apply to the vars listed in `.env.example`.

## Start

```bash
docker compose up --build
```

UI: `http://localhost:3000`

Linux real eBPF (canonical PRD path):

```bash
EBPF_SIMULATOR_MODE=false EBPF_PRIVILEGED=true \
  docker compose -f docker-compose.yml -f docker-compose.linux.yml up -d --build
```

See `docs/linux-ebpf.md`.

## Smoke

```bash
curl -i http://127.0.0.1:3000/
curl -i http://127.0.0.1:3000/health
curl -i http://127.0.0.1:8082/health
curl -i http://127.0.0.1:8082/metrics
curl -i -H 'X-Guest: 1' http://127.0.0.1:3000/api/session/new
curl -i http://127.0.0.1:9090/-/ready
curl -i http://127.0.0.1:3100/ready
```

Expect: HTML from `/`, `ok` from `/health`, Prometheus text from `/metrics`,
JSON with `session_id` from session new.

## Browser / WebSocket

1. Open `http://localhost:3000`, start a guest sandbox.
2. Run `echo hello` — terminal output and connected status.
3. Run `sleep 35` — exit code `124`, container still usable for the next command.
4. Save snapshot, end session, restore from history (signed-in history needs
   Keycloak).
5. AI panel streams from Ollama when the model is available; otherwise fallback
   text (not a hard failure). First model pull can take several minutes.
6. Sign in via OIDC when Keycloak is up (`http://127.0.0.1:8087`); guest still
   works without signing in.

## Observability

- Grafana: `http://127.0.0.1:3001` (admin / admin) — Entropy Overview
- Prometheus alerts: `docs/monitoring.md`

## Vite hybrid

```bash
docker compose up --build
cd frontend && npm install && npm run dev
```

Open `http://127.0.0.1:5175`. Proxy target defaults to `http://127.0.0.1:8082`.

## Auth UI

Keycloak is part of Compose. Frontend build args `VITE_OIDC_*` default to
`http://127.0.0.1:8087` / realm `entropy` / client `entropy-app`. Guest remains
available when OIDC is misconfigured or unreachable.
