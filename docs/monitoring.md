# Monitoring (Prometheus / Grafana / Loki)

Local and free-VM observability uses the Compose stack:

| Service | Host port | Role |
|---------|-----------|------|
| Prometheus | `9090` | Scrapes Session Manager `/metrics` |
| Grafana | `3001` | Dashboards (admin / admin by default) |
| Loki | `3100` | Log store |
| Promtail | (internal) | Ships Docker container logs to Loki |

CloudWatch exporters remain in the codebase for optional AWS ops but are not
required for the free PRD target.

## Metrics (Session Manager `/metrics`)

Prometheus text format, auth bypassed. Scraped every 15s (`prometheus.yml`).

| Metric | Meaning |
|--------|---------|
| `entropy_active_sessions` | Current non-terminated sessions |
| `entropy_total_commands` | Commands executed |
| `entropy_total_crashes` | Crash-class outcomes |
| `entropy_error_rate_percent` | Failed / total commands |
| `entropy_ai_explanation_latency_ms` | AI first-token latency |
| `entropy_ai_token_count` | AI tokens accounted |
| `entropy_container_cpu_percent` | Sandbox CPU sample |
| `entropy_container_memory_percent` | Sandbox memory sample |
| `entropy_snapshot_saves` | Successful snapshot saves |
| `entropy_session_creation_failures` | Failed session creates |

## Alerts

`prometheus-alerts.yml` loads into Prometheus:

- `EntropyHighErrorRate` — `entropy_error_rate_percent > 5` for 1m
- `EntropyHighAILatency` — `entropy_ai_explanation_latency_ms > 5000` for 1m

Inspect:

```bash
curl -sS 'http://127.0.0.1:9090/api/v1/alerts'
curl -sS 'http://127.0.0.1:9090/api/v1/rules'
```

## Grafana

Open `http://127.0.0.1:3001` (user `admin`, password `admin`).

Provisioning under `grafana/provisioning/`:

- Datasources: Prometheus, Loki
- Dashboard: **Entropy Overview** (error rate + session-manager logs)

## Logs (Loki)

Promtail discovers Compose containers via the Docker socket and labels streams
with `service` (Compose service name). Session Manager uses zap JSON logs.

```bash
# last ~2h of session-manager lines
end=$(date +%s); start=$((end-7200))
curl -sS "http://127.0.0.1:3100/loki/api/v1/query_range?query=%7Bservice%3D%22session-manager%22%7D&start=${start}000000000&end=${end}000000000&limit=5"
```

## Health endpoints

Session Manager, Airlock, AI Explainer, and eBPF Monitor expose `GET /health`.

## Alert smoke

Generate load or temporarily lower thresholds in `prometheus-alerts.yml`, then
confirm alerts move to `firing` in the Prometheus Alerts UI or `/api/v1/alerts`.
Restore thresholds after the check.
