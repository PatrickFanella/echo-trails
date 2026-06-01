# Runbooks

Operational runbooks for repeatable project workflows.

Add runbooks when a task needs exact steps, credential boundaries, rollback notes, or external service setup.

Current workflows:

- Use `make verify` before commits and PRs.
- Use `docs/runbooks/database-migrations.md` for Postgres/PostGIS migration apply, test, and rollback guidance.
- Use `docs/runbooks/deployment.md` for staging/prod-beta deployment and rollback templates.
- Use `docs/runbooks/backups.md` for backup policy and restore drill templates.
- Use `docs/runbooks/monitoring.md` for signal and alert templates.
- Use `.gitea/ISSUE_TEMPLATE/open-pilot-task.yaml` for agent-ready work.
- Keep generated iOS prototype code reference-only unless an issue explicitly scopes changes there.
