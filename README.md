# EchoTrails

Public place-stories for real-world locations.

Core promise: people leave short Echoes tied to real-world Places so others can discover the stories of those Places.

## Status

Full-feature invite-gated TestFlight release candidate. The generated top-level
`ios/` app remains reference-only; `apps/ios` is the authoritative application.

Current codebase:

- SwiftUI/SwiftData iOS 17+ app with durable background media creation
- transactional Go API backed by Postgres/PostGIS and private S3-compatible storage
- Redis Streams worker with media processing, ranking, APNs, privacy, and moderation jobs
- authenticated React admin console and minimal public TestFlight preview site
- generated OpenAPI path contracts and executable privacy/release gates
- Open Pilot-ready issue and PR templates

Beta platform:

- iOS TestFlight app
- custom Go REST API
- Postgres/PostGIS
- Redis jobs
- S3-compatible media storage/CDN
- React/Vite admin web
- minimal public preview web

## Quick start

```bash
make verify
```

Verification runs Go formatting/vet/tests, TypeScript lint/build/tests, dependency
audits, OpenAPI drift checks, privacy and moderation gates, migration up/down,
docs/secrets checks, the Swift package tests, and a generic-device iOS build on
macOS. Release evidence commands also include `make integration-test`,
`make release-scale-test`, `make release-processing-test`, `make restore-drill`,
and `make release-service-drills`.

## Local development services

Local Postgres/PostGIS, Redis, and S3-compatible object storage are defined in `infra/local/compose.yaml`:

```bash
docker compose -f infra/local/compose.yaml up -d
docker compose -f infra/local/compose.yaml down
```

The compose file uses non-secret development defaults only. Do not commit `.env` files, production credentials, signing keys, or staging/prod object-storage access keys. See `infra/local/README.md` for ports and connection strings.

## API contract

The release OpenAPI contract lives at `packages/openapi/openapi.yaml` and is the
source for generated path constants used by iOS and web clients.

## Target monorepo layout

- `apps/api` — Go REST API service.
- `apps/worker` — background worker service.
- `apps/ios` — clean iOS app rebuild target; the existing top-level `ios/` prototype remains reference-only.
- `apps/admin-web` — React/Vite admin web app target.
- `apps/public-web` — minimal public preview/legal/CTA web app target.
- `packages/openapi` — OpenAPI contract and generated client checks.
- `packages/shared-contracts` — shared contract/schema artifacts once generated.
- `infra/local` — local development infrastructure.
- `infra/deploy` — secret-free deployment scaffolding and docs.

## Web applications

The admin and public web shells live in separate React/Vite app directories:

- `apps/admin-web` — role-gated moderation and operations UI
- `apps/public-web` — minimal share preview, disclosure, and TestFlight CTA site

Install dependencies from the app directory with either npm or pnpm:

```bash
cd apps/admin-web
npm install
npm run dev
```

```bash
cd apps/public-web
pnpm install
pnpm dev
```

Root verification installs locked dependencies with `npm ci`, then runs each
application's lint, production build, and Vitest suite.

## API domain primitives

`apps/api/internal/locationproof` validates the scoped creation-token preflight for Echo creation: fresh proof, GPS accuracy threshold, selected Place radius/boundary, restricted Place rejection, ambiguous eligible Place rejection, trust/rate gate decisions, and opaque creation token issuance without exact private coordinates in the token string.

## Auth configuration

Sign in with Apple token verification requires `ECHOTRAILS_APPLE_CLIENT_ID`.
APNs provider authentication uses the separate team, key, bundle, and private-key
settings documented in `infra/deploy/env.example`.

Do not commit secret values or signing material.

## API

The sole API binary is `apps/api/cmd/api`. Its complete identity, Place,
creation, media, social, discovery, subscriptions, Activity/APNs, privacy, and
admin surface is documented in `packages/openapi/openapi.yaml`.

Local development uses the Postgres/PostGIS, Redis, SMTP, object-storage, and
moderation fixtures in `infra/local/compose.yaml`. Production binaries do not
contain in-memory or log-based email startup modes. Runtime identity is derived
only from opaque bearer sessions. Public profiles are pseudonymous and omit
private account fields.

## Key docs

- Glossary: `CONTEXT.md`
- Product requirements: `docs/PRD.md`
- Technical plan: `docs/technical-plan.md`
- Open Pilot readiness: `docs/open-pilot.md`
- Labels: `docs/labels.md`
- Database migration runbook: `docs/runbooks/database-migrations.md`
- Local development services: `infra/local/README.md`

## Open Pilot

This repository keeps the base Open Pilot issue template and PR template.

Agent-ready issues must include:

- `ready-for-agent`
- deterministic test command
- clear acceptance criteria
- explicit out-of-scope notes

Add `agent:queued` only after human review.

## License

Licensed under `GPL-3.0-or-later`. See [LICENSE](LICENSE).
