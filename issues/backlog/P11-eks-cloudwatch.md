# P11: VM deploy + OSS observability

Status: local_complete / blocked_free_stack (operator VM signal check)
Stage: P11
Requirements: 15, 16

## Goal
Deploy the split services on a free Linux VM and validate OSS monitoring:
Prometheus metrics, Grafana dashboards, and Loki log ingestion/alerting.

## Checklist
- [x] Prometheus metrics endpoint exposed and scraped (`/metrics` + `prometheus.yml`)
- [x] Grafana dashboards provisioned (Entropy Overview)
- [x] Loki + Promtail ingestion configured for container / zap JSON logs
- [x] Alert rules defined and observed firing locally (`prometheus-alerts.yml`)
- [ ] Operator: same signals validated on the target VM (optional tunnel)

## Exit criteria
- [x] Local monitoring shows live signals; sample alerts validate
- [ ] Operator: VM monitoring shows live signals
