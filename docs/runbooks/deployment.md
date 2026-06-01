# Deployment Runbook

## Status

Template only. No production deploy has been executed from this repository.

## Preconditions

- `make verify` passes on `main`.
- Secrets are provided by the host environment, not committed files.
- Database migrations have a rollback plan.
- Health endpoints `/health`, `/ready`, and `/metrics` are reachable after start.

## Deploy steps

1. Fetch the target commit on the staging host.
2. Load secret-free environment values from `infra/deploy/env.example` and host secrets from the approved secret store.
3. Run migrations against staging.
4. Start API, worker, public web, and admin web services.
5. Check `/ready`, `/metrics`, public web CTA, and admin shell fixture.

## Rollback

1. Stop new traffic.
2. Restore previous service image/commit.
3. Roll back migrations only if the matching down migration was tested.
4. Re-run smoke checks.

## Verification

- `make verify`
- staging smoke checklist in `docs/runbooks/testflight-beta.md`
