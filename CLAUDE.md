# CLAUDE.md — Working guide for agents in the Entropy repo

This file tells any AI agent (or new contributor) how to work in this repository. Read it fully before making changes.

## Read order at the start of every session
1. `STATUS.md` — the current stage, blockers, and the next action.
2. `ROADMAP.md` — the full stage list and each stage's exit criteria.
3. `memory/` — product intent, architecture, and conventions.
4. The matching `issues/backlog/PXX-*.md` for the active P-stage (legacy `SXX-*.md` is complete history).

## What Entropy is
Entropy is a chaos-engineering sandbox. A user starts an isolated Ubuntu
container (guest, or Cognito when configured), runs dangerous commands in a
real terminal, sees a system-health visualization, and receives AI root-cause
explanations when things fail. Guided scenarios, snapshot/restore, eBPF events,
and AWS packaging are implemented; live AWS acceptance is tracked in
`docs/acceptance-p12.md` and `docs/your-actions.md`.

## Non-negotiable rules
1. One stage per change set. Do not begin the next stage until the current stage's exit criteria pass.
2. No emojis anywhere: UI text, user-facing logs, AI panel chrome, README, issue templates, commit messages.
3. Do not invent additional backends beyond Session Manager, Airlock, AI Explainer, and eBPF Monitor. Extend those services or `pkg/`.
4. Update `STATUS.md` and append to `memory/session-log.md` at the end of every working session.
5. Complete the matching `issues/backlog/PXX-*.md` checklist for the stage.
6. Keep user-facing copy calm, technical, and short.
7. Never commit `node_modules/` or `frontend/dist/` (already gitignored).

## Port map (local development)
- Compose UI: `http://localhost:3000`
- Local Vite: `http://localhost:5175` (proxies `/api` and `/ws` to Session Manager)
- Session Manager on the host: `http://localhost:8082` (maps to container port `8080`)
- Keycloak: `http://localhost:8087`
- Grafana: `http://localhost:3001` — Prometheus: `9090` — Loki: `3100`
- Inside Compose only: Airlock `8081`, AI Explainer `8082`, eBPF Monitor `8083`
- Redis / Postgres / MinIO / Ollama: internal to Compose (not published on the host)

Do not confuse host `8082` (Session Manager) with Compose-internal AI Explainer `8082`.

## Known host conflicts
Other local stacks frequently occupy `6379` (Redis) and `8080`. Entropy avoids
publishing those on the host. If you see "port is already allocated" or an
unexpected app on `8080`, it is a different stack, not Entropy.

## Design direction
The UI theme is "Slate Studio": a premium light product chrome with a single dark surface (the terminal well). Full tokens live in `docs/ui.md`. Do not drift toward purple gradients, neon glow, emoji icons, rounded-full pill clusters, or a generic dark dashboard look.

## Branch
Work happens on `kalyan_dev`.
