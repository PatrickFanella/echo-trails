# Backup and Restore Runbook

## Status

Template only. No production backup or restore has been executed from this repository.

## Scope

- PostgreSQL/PostGIS database dumps.
- Media object bucket metadata and object inventory.
- Deployment config excluding secrets.

## Backup policy

- Daily logical DB backup during beta.
- Weekly restore drill before TestFlight expansion.
- Media inventory snapshot paired with DB backup timestamp.
- Retain raw analytics for at most 90 days unless aggregated.

## Restore drill

1. Create isolated restore database.
2. Restore latest DB dump.
3. Verify migrations table and seed smoke queries.
4. Compare media inventory counts.
5. Run API `/ready` against restored database.
6. Record timestamp, commit, backup ID, operator, and result.

## Rollback notes

Never overwrite production data during a drill. Use isolated resources only.
