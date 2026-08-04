#!/usr/bin/env bash
# Bootstrap Entropy on a Linux VM for public free-stack close-out.
# Run from the repo root on the VM (not on macOS for real eBPF).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if [[ "$(uname -s)" != "Linux" ]]; then
  echo "This bootstrap targets Linux. On macOS use: docker compose up --build" >&2
  exit 1
fi

if [[ ! -f .env ]]; then
  cp .env.example .env
  echo "Created .env from .env.example — edit OIDC and CLOUDFLARE_TUNNEL_TOKEN before tunnel step."
fi

# Ensure Linux eBPF defaults if unset
grep -q '^EBPF_SIMULATOR_MODE=' .env 2>/dev/null || echo 'EBPF_SIMULATOR_MODE=false' >> .env
grep -q '^EBPF_PRIVILEGED=' .env 2>/dev/null || echo 'EBPF_PRIVILEGED=true' >> .env
sed -i.bak 's/^EBPF_SIMULATOR_MODE=.*/EBPF_SIMULATOR_MODE=false/' .env
sed -i.bak 's/^EBPF_PRIVILEGED=.*/EBPF_PRIVILEGED=true/' .env
rm -f .env.bak

echo "Starting stack with Linux eBPF override..."
docker compose -f docker-compose.yml -f docker-compose.linux.yml up -d --build

echo "Warming Ollama model (may take several minutes)..."
MODEL="$(grep -E '^OLLAMA_MODEL=' .env 2>/dev/null | cut -d= -f2- || true)"
MODEL="${MODEL:-llama3}"
docker compose exec -T ollama ollama pull "$MODEL" || true

echo "Private smoke:"
curl -sf -o /dev/null -w 'frontend_health=%{http_code}\n' http://127.0.0.1:3000/health || true
curl -sf -o /dev/null -w 'session_manager=%{http_code}\n' http://127.0.0.1:8082/health || true
curl -sf -o /dev/null -w 'metrics=%{http_code}\n' http://127.0.0.1:8082/metrics || true

echo "eBPF mode:"
docker compose exec -T ebpf-monitor wget -qO- http://127.0.0.1:8083/health || true
echo

echo "Next: set CLOUDFLARE_TUNNEL_TOKEN in .env, wire app/auth hostnames in Cloudflare,"
echo "then: docker compose --profile tunnel up -d cloudflared"
echo "See docs/vm-deploy.md and docs/closeout-pasteback.md"
