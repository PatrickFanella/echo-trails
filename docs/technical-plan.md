# EchoTrails Technical Plan

## Architecture summary

EchoTrails will be rebuilt as a monorepo with a clean iOS app, custom Go REST API, Postgres/PostGIS data store, Redis-backed jobs, S3-compatible object storage/CDN, separate React/Vite admin app, and minimal public preview web app.

The existing generated iOS app is reference-only. Do not preserve its Xcode project, file structure, or simulated implementations.

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

Each target app, package, and infra directory has a placeholder README so follow-up issues can add implementation in focused PRs. The repository-level generated `ios/` prototype remains reference-only; new iOS work targets `apps/ios/` unless an issue explicitly says otherwise.

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
- `private_creation_location geography(Point)` encrypted or tightly access-controlled
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

The baseline machine-readable contract lives in `packages/openapi/openapi.yaml`. It starts with health, auth, place, and Echo paths and is validated by `packages/openapi/scripts/lint-openapi.py`. `npm run check` also regenerates TypeScript and Swift path constants and fails on drift as part of `make verify`.

Initial resource groups:

- `/auth/apple`
- `/auth/email/start`
- `/auth/email/verify`
- `/sessions`
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
- `/exports`
- `/account/delete`
- `/admin/*`

Every mutating mobile request should support idempotency keys.

Current scoped profile implementation target: `apps/api` provides standard-library Go handlers for `GET /me`, `PATCH /me`, and `GET /users/{handle}` backed by an in-memory profile store boundary. Handles are normalized, validated, unique, and include handle-change cooldown scaffolding. Public handle responses are pseudonymous and must not expose private account fields such as email, internal user IDs, session data, or handle-change state.

Current scoped location proof implementation target: `apps/api/internal/locationproof` contains pure Go validation for Echo creation-token preflight. It validates proof freshness, GPS accuracy, selected Place radius/boundary, restricted Place status, ambiguous eligible Places, and trust/rate gate decisions before issuing an opaque signed creation token. The token is bound to user, Place, proof time, issue time, and expiry, and must not expose exact private coordinates.

The scoped Sign in with Apple server boundary lives under `apps/api/internal/auth/apple`. It defines the Apple identity verifier interface, a deterministic test verifier, identity-linking service semantics, and production config placeholders. Production Apple verification must read only the secret names `ECHOTRAILS_APPLE_TEAM_ID`, `ECHOTRAILS_APPLE_CLIENT_ID`, `ECHOTRAILS_APPLE_KEY_ID`, and `ECHOTRAILS_APPLE_PRIVATE_KEY`; no secret values or signing material belong in git.

Current scoped implementation target: `apps/api` provides standard-library Go handlers for passwordless email auth and session refresh/logout; it is intentionally storage/provider-agnostic for later persistence swaps. The local dev email adapter is explicit opt-in only and production must configure a real provider before beta.

Current scoped web implementation target: `apps/admin-web` and `apps/public-web` provide minimal React/Vite placeholders. Each app documents npm and pnpm install paths, and its TypeScript lint/build/test scripts skip with an explicit message until dependencies are installed in that app directory.

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

Current scoped implementation target for #28/#29 adds API-only direct upload
handlers for `POST /media/uploads`, `GET /media/uploads/{uploadId}`, and
`POST /media/uploads/{uploadId}/complete`. The handlers validate media type,
content type, size, idempotency key replay, opaque object keys, and hide upload
URLs after completion/expiry. Returned upload URLs are local-dev placeholders
for MinIO, not production presigned URLs. The worker queue now has explicit enqueue,
reserve, retry/backoff, dead-letter, and Redis connection configuration
contracts.

Current scoped implementation target for #30 adds deterministic media processing
interfaces under `apps/worker/internal/media`. It models metadata-stripping
results, derivative plans for image thumbnails/previews, audio waveforms, and
video thumbnail/preview/waveform outputs, and uploaded/processing →
pending-review state transitions. The worker job processor can call this handler
for `process_media` jobs. Real Redis reservation, presigned-object verification,
FFmpeg/ImageMagick execution, transcription engines, and moderation scanners
remain later isolated adapters.

Current scoped implementation target for #31/#32 adds deterministic worker
contracts for `generate_transcript` and `scan_media_safety`. Transcription writes
best-effort transcript records for audio/video, skips images, and stores failures
without panicking jobs. Moderation is conservative: only explicit low-risk scanner
results are approved; unknown, medium, or scanner-offline states require review,
and high-risk results are rejected. Real Whisper/audio extraction, ML moderation,
and separate moderation hosts remain deferred adapters.

Current scoped implementation target for #33 extends media upload status with an
activity-safe lifecycle: `created`, `uploaded`, `processing`, `pending_review`,
`failed`, and `expired`, plus optional transcript/moderation status summaries.
Private upload URLs remain hidden after completion or expiry; no auth ownership,
notifications, or iOS Activity UI are included yet.

Current scoped iOS implementation target for #13 is a Swift Package shell under
`apps/ios` with stable app identity constants, generated OpenAPI path constants,
and Phase 3 tab names only. No auth, SwiftData, networking, map UI, or media UI
is included in the shell.

Current scoped implementation target for #34 keeps Phase 3 Linux-verifiable by
hardening the Swift Package baseline rather than requiring an Xcode project. It
adds pure Swift app bootstrap, local/staging/production environment selection,
API client configuration seams, and navigation descriptors. Runtime auth,
Keychain, SwiftData, networking, MapKit, media composer UI, and Xcode-only smoke
tests start in later Phase 3 issues.

Current scoped implementation target for #36 keeps navigation Linux-verifiable as
pure Swift data: route parsing, default selected tab, preferred tab metadata,
screen descriptors, accessibility label seeds, and auth-required route metadata.
The `ios-boundary-check` target verifies tab order, route contracts, and absence
of SwiftUI, MapKit, SwiftData, AuthenticationServices, and Security imports until
machine-specific UI/auth issues can be tested with Xcode.

Current scoped implementation target for #37-#43 closes Phase 3 as contract-only
Swift Package work on Linux. It adds descriptors/tests for Map markers and Place
detail, Explore search sections, Place-first Echo draft validation, SwiftData-ready
cache/outbox schema descriptors, media composer limits/upload intents, Profile and
Activity status rows, and accessibility audit surfaces. These contracts use only
Foundation and deterministic Python boundary checks; actual SwiftUI screens,
MapKit, SwiftData models, media pickers, runtime networking, Keychain/auth flows,
and Xcode UI smoke tests remain deferred to a macOS-capable implementation pass.

Current scoped implementation target for #35 finishes Phase 3 on Linux by adding
pure Swift auth/session contracts only: email code request/verify descriptors,
Apple token exchange descriptors, redacted session token descriptors, and a
Keychain-ready storage seam. It intentionally does not import
AuthenticationServices or Security, render login UI, persist tokens, or perform
runtime networking until a macOS/Xcode-capable pass can verify those integrations.

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

Current scoped implementation target for #44-#48 is a Linux-verifiable in-memory
API contract slice. It adds reaction totals with one reaction per user per Echo,
text-only one-level replies, stitch records linking new Echo IDs to source Echoes,
report/moderation intake records, viewer hides/user blocks that filter visible
replies and notification eligibility, and stored activity-safe notification
records. This does not add DB persistence, ranking, APNs delivery, abuse scoring,
admin adjudication, Reply screenshot rendering, or iOS social UI; those remain
separate Phase 4/5 implementation passes.

Current scoped implementation target for #49-#51 is a Linux-verifiable admin web
fixture workflow in `apps/admin-web`. It adds deterministic role-gated shell
navigation, moderation queue/action-intent placeholders, and Places review
placeholders for boundary-sensitive overrides. It does not add real auth, API
writes, DB persistence, sanctions, appeals, or production moderation decisions.

Current scoped implementation target for #52 is a Linux-verifiable iOS social UI
contract layer in `apps/ios`. It adds pure Swift descriptors for reaction bars,
reply composers/threads, stitch source cards, report flows, hide/block actions,
and authenticated action metadata. It does not add SwiftUI/UIKit views, runtime
networking, SwiftData persistence, screenshot rendering, APNs, or production
moderation decisions.

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
