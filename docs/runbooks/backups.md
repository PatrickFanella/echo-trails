# Backup and Restore Runbook

## Status

The disposable Postgres/PostGIS and private-object restore drill is executable with
`make restore-drill`. It writes operator, commit, backup digest, timestamp, and
result evidence to `.artifacts/release-drills/restore-drill.json`. A successful
run against the beta deployment remains a release requirement; local evidence
does not stand in for production backup execution.

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

For an engineering integration drill, run `make restore-drill`. The command:

1. Starts the disposable PostGIS and private object-storage fixtures.
2. Applies every migration and inserts relational, PostGIS, and media-object
   canaries.
3. Creates a custom-format logical backup and mirrors the private media bucket
   into an isolated backup bucket.
4. Restores the database and objects into fresh, isolated destinations.
5. Compares public schema and object inventory counts, verifies the restored
   object checksum, and queries the restored spatial canary.
6. Records timestamp, commit, backup digest, operator, and result.

Before TestFlight distribution, repeat the same procedure against the latest
beta backup in an isolated restore environment, point the API at that restored
database, and verify `/ready`. Attach the resulting evidence to the release
record.

## Rollback notes

Never overwrite production data during a drill. Use isolated resources only.
