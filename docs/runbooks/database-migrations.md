# Database migrations

EchoTrails database migrations live in `apps/api/db/migrations` as paired `*.up.sql` and `*.down.sql` files.

## Apply migrations locally

Run against a disposable Postgres database with PostGIS available:

```bash
DATABASE_URL='postgres://user:password@localhost:5432/echotrails_dev?sslmode=disable' \
  bash apps/api/scripts/migrate-up.sh
```

Each migration file is applied with `psql --single-transaction` and `ON_ERROR_STOP=1` so a failing migration does not leave a half-applied schema.

The migrate-up integration test is enabled when `ECHOTRAILS_TEST_DATABASE_URL` points at a throwaway Postgres/PostGIS database and `psql` is installed:

```bash
ECHOTRAILS_TEST_DATABASE_URL='postgres://user:password@localhost:5432/echotrails_test?sslmode=disable' \
  sh -c 'cd apps/api && go test ./internal/db/migrations'
```

`make verify` always runs migration unit checks plus embedded PGlite/PostGIS up/down SQL execution. The PGlite check asserts that down migrations remove core tables while retaining shared extensions. The live `psql` integration test is skipped unless the local database prerequisites are present.

## Rollback policy

- Down migrations are required for development and test databases so schema experiments can be reset safely.
- Production rollback is forward-only by default: restore from backup or ship a corrective migration instead of destructively rolling back user data.
- Destructive down migrations must only run against disposable/local databases or during an explicitly approved recovery procedure.
- `apps/api/scripts/migrate-down.sh` refuses to run unless `ECHOTRAILS_ALLOW_DESTRUCTIVE_MIGRATION_DOWN=1` is set for that command.
- The initial down migration drops tables created by the initial up migration but intentionally leaves shared database extensions such as PostGIS and citext installed.
