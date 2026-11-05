#!/usr/bin/env bash
# Starts Redis, Airlock, Session Manager, a fake AI Explainer, and Vite for
# end-to-end tests. Requires Docker and Go. Writes logs to $E2E_DIR.
# Usage: scripts/e2e/run-stack.sh start|stop
set -euo pipefail
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
E2E_DIR=${E2E_DIR:-$ROOT/.e2e}
mkdir -p "$E2E_DIR/bin"

stop() {
  for f in "$E2E_DIR"/*.pid; do
    [ -f "$f" ] || continue
    kill "$(cat "$f")" 2>/dev/null || true
    rm -f "$f"
  done
  docker ps -q --filter label=entropy.type=sandbox | xargs -r docker rm -f >/dev/null || true
  docker rm -f entropy-e2e-redis >/dev/null 2>&1 || true
}

wait_for() {
  for _ in $(seq 1 60); do
    if curl -sf "$1" >/dev/null; then return 0; fi
    sleep 1
  done
  echo "timed out waiting for $1" >&2
  exit 1
}

start() {
  stop
  docker run -d --name entropy-e2e-redis -p 127.0.0.1:6390:6379 redis:7-alpine >/dev/null
  (cd "$ROOT/services/airlock" && go build -o "$E2E_DIR/bin/airlock" .)
  (cd "$ROOT/services/session-manager" && go build -o "$E2E_DIR/bin/session-manager" .)
  export REDIS_URL=redis://127.0.0.1:6390
  PORT=8081 SANDBOX_RUNTIME=${SANDBOX_RUNTIME:-auto} SANDBOX_IMAGE=${SANDBOX_IMAGE:-} "$E2E_DIR/bin/airlock" >"$E2E_DIR/airlock.log" 2>&1 </dev/null &
  echo $! >"$E2E_DIR/airlock.pid"
  python3 "$ROOT/scripts/e2e/fake-ai.py" "$E2E_DIR/ai-requests.log" 8093 >"$E2E_DIR/fake-ai.log" 2>&1 </dev/null &
  echo $! >"$E2E_DIR/fake-ai.pid"
  PORT=8082 AIRLOCK_URL=http://127.0.0.1:8081 AI_EXPLAINER_URL=http://127.0.0.1:8093 \
    "$E2E_DIR/bin/session-manager" >"$E2E_DIR/session-manager.log" 2>&1 </dev/null &
  echo $! >"$E2E_DIR/session-manager.pid"
  (cd "$ROOT/frontend" && exec npx vite --host 127.0.0.1 --port 5175) >"$E2E_DIR/vite.log" 2>&1 </dev/null &
  echo $! >"$E2E_DIR/vite.pid"
  wait_for http://127.0.0.1:8081/health
  wait_for http://127.0.0.1:8082/health
  wait_for http://127.0.0.1:5175/
  echo "e2e stack ready (logs in $E2E_DIR)"
}

case "${1:-start}" in
  start) start ;;
  stop) stop ;;
  *) echo "usage: $0 start|stop" >&2; exit 2 ;;
esac
