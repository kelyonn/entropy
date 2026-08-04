# Production inputs (one at a time)

Local free-stack Compose is implemented. Live public acceptance needs the
operator inputs below **in order**. Paste them via
[`docs/closeout-pasteback.md`](closeout-pasteback.md). After each item, the
agent (or you) can run the next step from `docs/vm-deploy.md` and
`docs/your-actions.md`.

| # | You provide | Next step |
|---|-------------|-----------|
| 1 | Linux VM with Docker + Compose (SSH or console) | Clone repo; `bash scripts/vm-bootstrap.sh` |
| 2 | Confirm BPF-capable kernel (or accept simulator-only until ready) | Enable `docker-compose.linux.yml` + privileged eBPF |
| 3 | Disk budget for Ollama model pull | `ollama pull` chosen model |
| 4 | Cloudflare account + domain (or quick-tunnel for demo) | Create named tunnel |
| 5 | `CLOUDFLARE_TUNNEL_TOKEN` | `docker compose --profile tunnel up -d cloudflared` |
| 6 | Chosen public hostnames (`app`, `auth`) | Wire tunnel public hostnames + DNS |
| 7 | Updated Keycloak redirect URIs for public UI origin | Rebuild frontend with `VITE_OIDC_*` |
| 8 | Live acceptance window | Run P12 checks; update `STATUS.md` |

**Not required for local learning use:** items 1–8. See `docs/local-ops.md`.

**Cost:** all listed components have free tiers or self-hosted zero-license cost.
You still pay for whatever VM/disk you choose (including “always free” quotas).
