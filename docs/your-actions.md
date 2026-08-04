# What you need to do (free-stack production)

**Sequential handoff:** see [`docs/production-inputs.md`](production-inputs.md).
**Paste-back template for close-out:** [`docs/closeout-pasteback.md`](closeout-pasteback.md).

Local Compose already runs the full free stack (Keycloak, Ollama, Prometheus,
Grafana, Loki, MinIO, Postgres, Redis, four services, frontend). The items
below are operator steps on a Linux VM plus public exposure.

Full VM runbook: [`docs/vm-deploy.md`](vm-deploy.md).
Linux eBPF: [`docs/linux-ebpf.md`](linux-ebpf.md).
VM one-shot script (Linux only): `scripts/vm-bootstrap.sh`.

## 1. Linux host with Docker

Provide SSH (or console) access to a Linux VM with Docker Engine and Compose
plugin installed. Confirm:

```bash
docker version
docker compose version
uname -r
```

Paste results using [`docs/closeout-pasteback.md`](closeout-pasteback.md).

## 2. Clone and boot Compose

```bash
git clone <repo> && cd entropy
cp .env.example .env
# Set EBPF_SIMULATOR_MODE=false and EBPF_PRIVILEGED=true on Linux
docker compose -f docker-compose.yml -f docker-compose.linux.yml up -d --build
```

Smoke: `docs/local-ops.md` against the VM’s published ports (or localhost via SSH
tunnel).

## 3. Ollama model

```bash
docker compose exec ollama ollama pull llama3
```

Confirm an AI explanation streams in the UI (not only fallback text).

## 4. Keycloak public URLs

After you know the public hostnames:

1. Add UI origin to the `entropy-app` client redirect URIs.
2. Set `VITE_OIDC_URL` / `OIDC_ISSUER_URL` for that auth hostname.
3. Rebuild frontend: `docker compose up -d --build frontend`.

Test user from realm import: `user@example.com` / `entropy123` (change in real
deployments).

## 5. Cloudflare Tunnel + DNS

1. Create a named tunnel; copy the token into `.env` as `CLOUDFLARE_TUNNEL_TOKEN`.
2. Map `app.` → `http://frontend:80` and `auth.` → `http://keycloak:8080`.
3. Start: `docker compose --profile tunnel up -d cloudflared`.
4. Confirm `https://app...` loads and WebSocket sessions work (`wss://`).

Details: `docs/vm-deploy.md`.

## 6. Linux eBPF smoke

Follow `docs/linux-ebpf.md`. Logs must show `eBPF probes attached`. Confirm a
kernel/failure event reaches viz + AI.

## 7. Observability smoke

- Prometheus: `http://<host>:9090` (or tunnel) scrapes `entropy_*` metrics.
- Grafana: `http://<host>:3001` — Entropy Overview dashboard.
- Alerts: `docs/monitoring.md`.

## 8. Acceptance

Run the learner flows in `docs/acceptance-p12.md`, then update `STATUS.md` to
`stage_state: complete` / `next_stage: none` and finish remaining boxes in
`issues/backlog/P12-amplify-acceptance.md`.

## Local (already available; no VM required)

```bash
docker compose up --build
# UI: http://localhost:3000
# Grafana: http://localhost:3001
# Prometheus: http://localhost:9090
# Keycloak: http://localhost:8087
# Session Manager: http://127.0.0.1:8082
```
