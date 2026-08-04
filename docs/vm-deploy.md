# Free VM deploy (Docker Compose + Cloudflare Tunnel)

Target: one Linux VM running the full Entropy Compose stack, exposed to learners
over HTTPS/WSS via a free Cloudflare Tunnel. No paid AWS control plane required.

## What you need

1. A Linux VM (always-free cloud VM, home lab, or similar) with Docker + Compose.
2. A free [Cloudflare](https://www.cloudflare.com/) account and a domain on
   Cloudflare (or a trycloudflare quick tunnel for temporary demos).
3. Enough disk for Postgres, MinIO, Ollama model weights, and Loki/Prometheus
   data (plan for tens of GB if you pull a full LLM).

## Bootstrap

```bash
# On the VM
git clone <your-entropy-repo> && cd entropy
cp .env.example .env
# Edit .env: OIDC URLs for public hostname, Ollama model, eBPF Linux flags
```

Recommended production-ish `.env` knobs:

```bash
EBPF_SIMULATOR_MODE=false
EBPF_PRIVILEGED=true
OLLAMA_MODEL=llama3
# After tunnel hostname is known:
# VITE_OIDC_URL=https://auth.example.com
# OIDC_ISSUER_URL=https://auth.example.com/realms/entropy
```

Bring the stack up:

```bash
docker compose -f docker-compose.yml -f docker-compose.linux.yml up -d --build
```

On macOS or without BPF privileges, omit `docker-compose.linux.yml` and keep
the simulator defaults.

### First-time Ollama model

Ollama serves immediately; the first AI request may pull the model (can take
minutes). Optional warm pull:

```bash
docker compose exec ollama ollama pull llama3
```

### Keycloak

Realm import: `keycloak/entropy-realm.json` (client `entropy-app`, test user
`user@example.com` / `entropy123`). Admin UI: host port `8087` (local) or via
tunnel hostname for `auth`.

Update Keycloak client redirect URIs to include your public UI origin
(`https://app.example.com/*`) before relying on OIDC sign-in from the tunnel.

## Persistent volumes

Compose volumes: `pgdata`, `miniodata`, `snapshotdata`, `ollamadata`. Back them
up with normal volume export / VM snapshots. Treat MinIO and Postgres as the
learner data plane.

## Cloudflare Tunnel

### Option A — Named tunnel (recommended)

1. In Cloudflare Zero Trust → Networks → Tunnels, create a tunnel and copy the
   token.
2. Set in `.env`:

```bash
CLOUDFLARE_TUNNEL_TOKEN=eyJ...
```

3. Start the tunnel sidecar:

```bash
docker compose --profile tunnel up -d cloudflared
```

4. In the tunnel Public Hostname config, map:

| Hostname | Service |
|----------|---------|
| `app.example.com` | `http://frontend:80` |
| `auth.example.com` | `http://keycloak:8080` |

WebSocket paths (`/ws`) go through the same `app` hostname as the UI nginx
proxy. Rebuild the frontend after setting public `VITE_OIDC_*` and (if needed)
empty same-origin `VITE_API_URL` / `VITE_WS_URL` so the browser uses
`https://` / `wss://` on the app host.

### Option B — Quick tunnel (ephemeral demo)

```bash
docker run --rm --network entropy_default cloudflare/cloudflared:latest \
  tunnel --url http://frontend:80
```

Note the printed `*.trycloudflare.com` URL. Keycloak redirects will need that
origin added to the client; this is for smoke demos only.

## HTTPS / WSS

Cloudflare terminates TLS at the edge. Learners open `https://app.example.com`.
Nginx in `frontend` proxies `/api` and `/ws` to Session Manager on the Compose
network. Ensure the tunnel does not strip WebSocket upgrade headers (default
named-tunnel HTTP service does not).

## Update / restart

```bash
git pull
docker compose -f docker-compose.yml -f docker-compose.linux.yml up -d --build
docker compose --profile tunnel up -d cloudflared
```

Rolling restarts: recreate one service at a time if you need to limit downtime.
After Session Manager recreate, restart `frontend` if nginx cached a stale
upstream IP (`docker compose restart frontend`).

## Operator checklist

See `docs/your-actions.md` and `docs/production-inputs.md` for the ordered
hand-off list (VM access, tunnel token, DNS hostnames, Keycloak redirects,
Linux eBPF smoke, acceptance).
