# EchoTrails Technical Plan

## Architecture summary

EchoTrails is implemented as a monorepo with a clean iOS app, custom Go REST API, Postgres/PostGIS data store, Redis-backed jobs, S3-compatible object storage, separate React/Vite admin app, and minimal public preview web app.

The existing top-level generated iOS app is reference-only and non-authoritative. Release implementation lives under `apps/ios`; do not add release behavior to the legacy prototype.

## Current release implementation status

The API, worker, migrations, moderation service, admin/public web applications,
and SwiftUI iOS runtime now exist in their target directories. Historical phase
notes later in this document describe the incremental state when those slices
landed; they are not current claims that the applications remain placeholders
or contract-only. `docs/runbooks/testflight-beta.md` is the authoritative list
of remaining release validation and external TestFlight operations.

## Repository layout

Target layout:

```text
/
├── CONTEXT.md
├── docs/
│   ├── PRD.md
│   ├── technical-plan.md
│   └── superpowers/plans/
├── apps/
│   ├── api/
│   ├── worker/
│   ├── ios/
│   ├── admin-web/
│   └── public-web/
├── packages/
│   ├── openapi/
│   └── shared-contracts/
└── infra/
    ├── local/
    └── deploy/
```

The repository-level generated `ios/` prototype remains reference-only; all release iOS work targets `apps/ios/`.

## Core stack decisions

- API: Go REST/JSON.
- API contract: `packages/openapi/openapi.yaml`, generated/checked clients for Swift and TypeScript.
- Database: Postgres + PostGIS.
- Search: Postgres full-text first.
- Queue: Redis queue.
- Workers: separate worker process/container from same codebase.
- Media: S3-compatible object storage + CDN.
- iOS persistence: SwiftData cache/outbox/settings; server source of truth.
- Admin web: React + Vite, separate from public web.
- Public web: minimal share preview/legal/CTA app.
- Auth: Sign in with Apple + passwordless email code/universal link.
- Push: APNs from worker.
- Analytics: first-party Postgres append-only event table.

## Data model outline

Use UUIDv7 public IDs. Server timestamps are authoritative.

Primary tables/entities:

- `users`
- `user_profiles`
- `sessions`
- `devices`
- `auth_identities`
- `consents`
- `places`
- `place_provider_mappings`
- `place_aliases`
- `place_subscriptions`
- `echoes`
- `echo_links`
- `replies`
- `media_assets`
- `media_derivatives`
- `reactions`
- `bookmarks`
- `reports`
- `moderation_actions`
- `appeals`
- `blocks`
- `notifications`
- `analytics_events`
- `audit_logs`
- `invite_codes`
- `idempotency_keys`
- `jobs`

### Echo schema notes

`echoes` should include:

- `id uuidv7 primary key`
- `author_user_id`
- `place_id`
- `title`
- `content_mode`: `text`, `audio`, `photo_set`, `video`
- `body_text` for text mode
- `caption_text` for media modes
- `status`: draft/uploading/processing/pending_review/published/limited/removed/deleted/failed
- `public_location geography(Point)` or geometry point for jittered public map display
- exact creation coordinates stored only as application-encrypted ciphertext in the restricted `creation_authorizations` table
- `location_proof_captured_at`
- `location_accuracy_meters`
- `creation_token_id`
- `sensitive_label`
- `created_at`, `published_at`, `updated_at`, `deleted_at`

### Reply schema notes

`replies` should be separate from `echoes`.

- Text-only.
- No title.
- `echo_id` target.
- Optional `parent_reply_id`, max one child level.
- `body_text` max 500 chars.
- Edit history stored separately or via revision table.

### Stitch schema notes

Stitches are Echoes with a row in `echo_links`.

`echo_links` should include:

- `source_echo_id`
- `target_echo_id`
- `link_type = stitch`
- optional quoted reply reference metadata
- optional rendered quote snapshot media asset ID

Public API exposes this as `stitches`; DB remains flexible with `echo_links`.

## API shape

Use cursor pagination for feeds/search/lists.

Shared API primitives live under `apps/api/internal/primitives` and establish the first reusable server semantics:

- UUIDv7 public IDs with timestamp ordering.
- UTC server timestamp normalization.
- Opaque cursor encoding/decoding using timestamp plus UUID tie-breaker.
- Idempotency key replay semantics with request fingerprint protection.
- Structured JSON error envelopes shaped as `{ "error": { "code", "title", "detail", "meta" } }`.

The baseline machine-readable contract lives in `packages/openapi/openapi.yaml`. It starts with health, auth, place, and Echo paths and is validated by `packages/openapi/scripts/lint-openapi.py`. `npm run check` also regenerates TypeScript and Swift operation path builders and fails on drift as part of `make verify`; iOS and admin runtime-source checks reject handwritten API routes.

Initial resource groups:

- `/auth/apple`
- `/auth/email/start`
- `/auth/email/verify`
- `/sessions`
- `/account/bootstrap`
- `/me`
- `/users/{handle}`
- `/places/search`
- `/places/{placeId}`
- `/places/{placeId}/echoes`
- `/discovery/nearby`
- `/places/{placeId}/subscribe`
- `/places/requests`
- `/echoes`
- `/echoes/{echoId}`
- `/echoes/{echoId}/reactions`
- `/echoes/{echoId}/replies`
- `/echoes/{echoId}/stitches`
- `/media/uploads`
- `/media/uploads/{uploadId}/complete`
- `/reports`
- `/blocks`
- `/bookmarks`
- `/notifications`
- `/notification-preferences`
- `/analytics/opt-out`
- `/exports`
- `/account/delete`
- `/admin/*`

Every mutating mobile request should support idempotency keys.

Profiles, handle cooldowns, and moderated avatars are backed by Postgres in the
production binary. Public handle responses remain pseudonymous and omit email,
internal user IDs, session data, and handle-change state.

Current scoped location proof implementation target: `apps/api/internal/locationproof` contains pure Go validation for Echo creation-token preflight. It validates proof freshness, GPS accuracy, selected Place radius/boundary, restricted Place status, ambiguous eligible Places, and trust/rate gate decisions before issuing an opaque signed creation token. The token is bound to user, Place, proof time, issue time, and expiry, and must not expose exact private coordinates.

The Sign in with Apple boundary under `apps/api/internal/auth/apple` verifies
issuer, audience, signature, expiry, and nonce-compatible identity linkage using
Apple's JWKS. Production requires `ECHOTRAILS_APPLE_CLIENT_ID`; APNs signing
credentials remain a separate worker concern and never belong in git.

Passwordless email requests, rotating refresh sessions, replay-safe refresh,
revocation, and identity links are persisted in Postgres. Production startup
requires SMTP; the development email adapter is available only with the explicit
memory-store switch.

`apps/admin-web` provides the authenticated moderation/operations console;
`apps/public-web` provides the intentionally narrow TestFlight/disclosure site.
Both use locked dependencies and are built/tested by `make verify`.

## Upload and publish flow

1. Client selects/validates Place.
2. Client obtains fresh location proof.
3. API validates auth, trust/rate limit, Place status, boundary, proof, and returns creation token.
4. API creates draft Echo + media records.
5. API returns presigned private upload URLs.
6. Client uploads directly to object storage.
7. Client finalizes upload with API.
8. Worker processes media:
   - strip metadata
   - normalize/transcode/compress
   - generate thumbnails/waveforms
   - generate transcripts/captions where possible
   - local conservative moderation scan
9. If safe: publish and create public derivatives/CDN URLs.
10. If uncertain/risky: keep pending/limited for moderator review.
11. If failed: show status and retry/delete options in Activity outbox.

## Moderation architecture

Constraints:

- Must use free/local/open-source moderation.
- Separate moderation host preferred.
- Fail closed for media.

Components:

- API service for content state transitions and admin actions.
- Worker service for queue processing.
- Moderation worker host for local scanning/transcription.
- Admin web for review/appeals/actions.
- Audit log for every privileged/safety action.

If moderation host offline:

- Media remains pending.
- Admin alert fires.
- Text can publish only if text scanner is available.

## Infra/deploy plan

Beta hosting:

- Public API on `almaz` homelab public edge host.
- Use actual homelab location in privacy disclosures.
- Separate moderation host.
- Avoid Kubernetes.
- Use containers with explicit resource limits.

Services:

- Go API.
- Go worker.
- Postgres/PostGIS.
- Redis.
- S3-compatible object storage/CDN.
- Admin web.
- Public web.
- Moderation worker service.
- Monitoring/exporters.

Environments:

- local dev
- staging
- prod-beta

Local development dependencies are scaffolded in `infra/local/compose.yaml` for disposable Postgres/PostGIS, Redis, and MinIO S3-compatible object storage. The checked-in compose values are non-secret development defaults only and must not be reused for staging or production. Local `.env` files, production credentials, signing material, and object-storage keys must stay out of git.

Backups before beta testers:

- Postgres PITR/daily backup.
- Media/object storage backup/versioning.
- Config/secrets backup.
- Restore drill.
- Redis queue persistence or documented acceptable loss behavior.

Observability:

- Structured logs.
- Health/readiness endpoints.
- Prometheus metrics.
- Error tracking.
- Job queue metrics/dashboard.
- Uptime checks.
- ntfy/Alertmanager alerts.

## CI and quality gates

Blocking checks:

- `make verify` is the local and CI baseline gate.
- Required docs, Open Pilot templates, and common committed secret filenames.
- Local Docker Compose config validation for Postgres/PostGIS, Redis, and object storage, with a lightweight Linux-safe fallback when Docker is unavailable.
- Go formatting, vet, and tests when `apps/api` or `apps/worker` modules are scaffolded.
- OpenAPI contract presence/lint checks when `packages/openapi` is scaffolded.
- TypeScript lint, build, and tests when web app package manifests are scaffolded.
- Swift/Xcode tests on macOS runners only; Linux runners skip Swift checks with an explicit message.
- SQL migration pairing/static checks plus embedded PGlite/PostGIS up/down execution when migration tooling is scaffolded; live Postgres/PostGIS migrate-up coverage when a throwaway database is supplied.

Testing requirements before beta:

- API unit tests.
- API integration tests with Postgres/Redis.
- Media pipeline tests.
- Moderation flow tests.
- iOS smoke/UI happy paths.
- Admin moderation workflow tests.

## Migration/build phases

### Phase 0: Repo cleanup and scaffolding

- Create monorepo layout.
- Move or delete broken generated artifacts.
- Keep current iOS code only as reference until replacement exists.
- Add docs, CI skeleton, local dev compose, lint/test commands.

### Phase 1: Domain/API foundation

- Core Postgres/PostGIS migrations for users, profiles, sessions, devices, identities, consents, places, Echoes, replies, media, moderation, analytics, audit, jobs, and idempotency.
- OpenAPI skeleton.
- Auth flows.
- Place search/request model.
- Creation token and location proof validation.

### Phase 2: Media and jobs

- Presigned uploads.
- Redis workers.
- Media processing.
- Local moderation integration.
- Transcripts/waveforms/thumbnails.
- Pending/status lifecycle.

Phase 2A/B skeleton includes migration `apps/api/db/migrations/0002_media_lifecycle_spine.*.sql`
for upload lifecycle, transcripts, and moderation reviews, plus a deterministic
worker job skeleton under `apps/worker` for `process_media`,
`generate_transcript`, and `scan_media_safety`.

The production path uses private presigned uploads, Postgres-authoritative media
records, Redis Streams reservation/retry/dead-letter handling, object verification,
ClamAV, FFmpeg/ImageMagick normalization, metadata stripping, derivatives,
faster-whisper transcripts, and conservative revision-pinned ShieldGemma/
ShieldGemma 2/OpenNSFW2 moderation. Unknown or failed scans remain private.

`apps/ios` is the authoritative SwiftUI/SwiftData application. It includes
Keychain sessions, Map/Explore/Activity/Profile, Place-first GPS proof, every
content mode, file-backed background transfers, durable pre-draft checkpoints,
social/safety flows, subscriptions, APNs, public profiles, avatars, appeals,
server-authoritative invite/consent/profile gating, cached offline discovery,
an outbox/upload/processing status dashboard, and accessible media playback. The portable Swift package
continues to provide generated contracts and deterministic Linux tests.

### Phase 3: iOS clean rebuild

- New SwiftUI project.
- System light/dark civic map journal design.
- Auth, tabs, Map, Explore, Activity, Profile.
- SwiftData outbox/cache.
- Place-first creation flow.
- Media capture/library upload.

### Phase 4: Social and safety

- Reactions.
- Replies with one-level nesting.
- Stitches with source cards and Reply screenshot snapshots.
- Reports/block/hide.
- Notifications.
- Admin moderation app.

Social and safety state is transactional in Postgres: replaceable reactions,
one-level editable Reply threads and revisions, full-content Stitches, immutable
worker-rendered Reply snapshots, visibility controls, sanctions, appeals,
notifications, and snapshot suppression for privacy or moderator safety removal.
The admin web uses role-checked HTTP-only sessions, CSRF, signed private previews,
audited confirmations/reasons, Place merge/boundary tools, invites, and job queues.

### Phase 5: Beta readiness

- Invite codes.
- Policy/legal pages.
- GDPR-grade export/delete/consent.
- Backups/restore drill.
- Monitoring/alerts.
- TestFlight beta.

Current scoped implementation target for #53-#62 is a Linux-verifiable beta
readiness package. It includes public web share/CTA/policy fixtures, consent and
privacy/analytics/beta access API contracts, deployment/backup/monitoring
runbooks, a pre-beta threat model, and a TestFlight checklist. It does not claim
legal approval, GDPR certification, live deployment, live backups, live alerts,
Apple signing, App Store Connect work, or a real TestFlight upload.

## Immediate implementation plan

First implementation should be scaffolding only, not feature work.

1. Create `apps/api`, `apps/worker`, `apps/ios`, `apps/admin-web`, `apps/public-web`, `packages/openapi`, `infra/local`.
2. Add minimal Go module for API and worker.
3. Add local compose for Postgres/PostGIS and Redis.
4. Add placeholder OpenAPI file.
5. Add React/Vite placeholders for admin/public web.
6. Create clean iOS app shell or placeholder directory depending local tooling availability.
7. Replace root invalid `PLAN.md` with pointer to `docs/PRD.md` and `docs/technical-plan.md`.
8. Remove invalid `rork.json` only if no Rork tooling depends on it; otherwise replace with valid minimal metadata.
9. Remove extraction-failed test artifacts.
10. Add CI skeleton and README commands.

## Architecture decision records

Initial ADRs live in `docs/adr/` and document durable decisions from this plan. ADRs explain the decision context and consequences; this technical plan remains canonical for target architecture and implementation sequencing.

## Known risks

- All media types in v1 greatly increases moderation/storage/UX scope.
- Free local moderation is likely less accurate and slower than paid APIs.
- Self-hosted public social beta has uptime/security/abuse risks.
- US+EU users with non-EU homelab hosting need careful disclosure and controls.
- Place provider-agnostic model slows MVP but prevents provider lock-in.
- Top-level product scope is large; phased delivery must stay strict.
