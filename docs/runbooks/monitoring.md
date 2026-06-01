# Monitoring and Alerting Runbook

## Status

Template only. No production dashboards or alerts are active from this repository.

## Signals

- API readiness: `/ready`
- API health: `/health`
- Prometheus metrics: `/metrics`
- Worker job success/failure counts when real queue adapter lands.
- Public/admin web availability.

## Alerts

- API `/ready` failing for 5 minutes.
- Error rate above beta threshold for 10 minutes.
- Backup job missing expected daily window.
- Restore drill overdue.
- Media worker dead-letter queue non-empty after processor adapters land.

## Dashboard fixture

Use `infra/deploy/monitoring-dashboard.fixture.json` as the dashboard contract until a real monitoring backend is selected.

## Privacy

Logs and metrics must not include raw access tokens, refresh tokens, exact private coordinates, email auth codes, or unredacted report details.
