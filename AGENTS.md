# AGENTS.md

Start with `CLAUDE.md`. It is the source of truth for how to work in this repo. This file is the short list of non-negotiables.

1. Read `STATUS.md`, then `ROADMAP.md`, then `memory/`, then the current stage's backlog file before coding.
2. One stage per change set. Do not start the next stage until exit criteria pass.
3. No emojis anywhere (UI, logs, docs, issues, commits).
4. Do not invent additional backends beyond Session Manager, Airlock, AI Explainer, and eBPF Monitor. Extend those services or `pkg/`.
5. Keep user-facing copy calm, technical, and short.
6. UI follows the Slate Studio theme in `docs/ui.md` (light chrome, dark terminal only).
7. Local ports: UI `3000` (Compose) or `5175` (Vite), Session Manager host `8082`, Keycloak `8087`, Grafana `3001`, Prometheus `9090`, Redis internal.
8. Never commit `node_modules/` or `frontend/dist/`.
9. Update `STATUS.md` and append `memory/session-log.md` at the end of each session.
10. Complete the matching backlog checklist for the stage (`PXX`; `SXX` is legacy).
11. Prefer editing existing files over creating parallel structures.
12. Verify runtime changes locally (Compose; see `docs/local-ops.md`) before marking a stage done.
13. Production operator steps live in `docs/your-actions.md` (free VM + Cloudflare Tunnel).
