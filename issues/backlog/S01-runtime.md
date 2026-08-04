# S01: Stabilize local runtime

Status: done
Stage: S1

## Goal
Boot Entropy locally without port fights or a blank page from a missing backend.

## Checklist
- [x] docker-compose.yml: redis has no host port publish
- [x] docker-compose.yml: backend has no host port; env REDIS_URL set
- [x] docker-compose.yml: frontend maps 3000:80, depends_on backend
- [x] docker-compose.yml: backend healthcheck on /health
- [x] .env.example with REDIS_URL, PORT, AWS_REGION, VITE_API_URL, VITE_WS_URL
- [x] frontend/vite.config.js proxy /api and /ws to 127.0.0.1:8082 confirmed
- [x] README verification section with exact curl checks
- [x] .gitignore keeps node_modules and dist untracked

## Exit criteria
- [x] Compose UI loads styled (not unstyled white)
- [x] Session create returns JSON with session_id
- [x] Port conflicts (6379, 8080) documented in README
