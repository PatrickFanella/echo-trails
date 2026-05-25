# EchoTrails Product Requirements

## Product promise

EchoTrails is a public place-stories product. People leave short Echoes tied to real-world Places so others can discover the stories of those Places.

The product is place-first, not people-first. Public profiles and social interactions exist to support Place discovery, not to create a generic follower feed.

## Target beta

- Platform: iOS via TestFlight.
- Backend: custom API.
- Distribution: invite-code gated beta.
- Region: US + EU users may join, but infrastructure uses actual homelab hosting location with explicit disclosure and GDPR-grade controls before EU invites.
- Rollout: selected cities first.
- Product success metric: Place engagement, measured by Echo detail views and reaction/reply/stitch rates per active user.

## Core users

- Public beta users age 13+.
- Users may be pseudonymous.
- Public Echoes are always attributed to a stable public handle.
- No truly anonymous public posting in v1.

## Core content model

### Echoes

An Echo is public user-created content attached to a Place.

Rules:

- Required title: 80 characters max.
- One content mode per Echo:
  - Text: title + required body, 500 characters max.
  - Audio: title + audio, optional caption.
  - Photo set: title + 1-5 photos, optional caption.
  - Video: title + video, optional caption.
- Media captions use the 500 character text limit.
- Echoes remain indefinitely unless deleted, removed, or restricted.
- Old high-quality Echoes may mix into main feeds using small capped evergreen inserts.

### Stitches

A Stitch is an Echo linked as a response to another Echo or Stitch.

Rules:

- Same title/body/media rules as Echoes.
- Must be created within the allowed boundary of its own Place.
- Appears independently in map/nearby/trending/place discovery.
- Shows source context linking back to the source Echo/Stitch.
- Can quote/import a Reply as a rendered screenshot, but formally targets the parent Echo/Stitch.
- Reply screenshot snapshots remain unchanged if the original Reply is edited/deleted.
- User gets a once-per-account warning before creating a Stitch from a Reply screenshot.

### Replies

A Reply is a text-only comment under an Echo or Stitch.

Rules:

- No title.
- 500 characters max.
- Can be made from anywhere.
- Does not appear in Place discovery.
- Supports one-level nested replies only.
- Oldest-first sorting.
- Thread-only @mentions, with notifications only when eligible by access/block/settings.
- Editable with history and visible edited marker.
- First Reply shows a once-per-account warning that public Replies may be quoted into Stitches.

### Reactions

- Current six reaction types: heart, fire, sad, laugh, clap, mind blown.
- One total reaction per user per Echo.
- Ranking uses filtered/trust-weighted reactions, not raw counts.

## Places

Places are named real-world points, venues, landmarks, addresses, or areas with coordinates and a human-readable name.

Rules:

- No arbitrary unnamed map pixels.
- Provider-agnostic internal Place IDs.
- External provider mappings stored on Place records.
- Users may request missing Places.
- Requested Places stay pending until reviewed.
- Echo drafts attached to rejected/merged Places remain in outbox so users can choose an approved/merged Place or discard.
- Places support point + optional boundary geometry.
- Admins can edit boundaries/radii with audit log.
- Place states: active, pending review, merged, restricted, removed, rejected.
- Duplicate Places merge via aliases/redirects so Echoes and subscriptions remain valid.
- Sensitive Place restrictions use rules + provider/category heuristics + admin overrides + user reports.
- Private residences are not allowed as public Places.
- Schools/minors-sensitive Places use stricter rules rather than normal treatment.

## Location rules

- Public map uses jittered/approximate Public Echo Location.
- Exact creation coordinate is stored privately for safety, abuse investigation, place matching, and debugging.
- Exact coordinates are never included in public API responses.
- Admin/moderator reveal of exact coordinates requires explicit reveal action, reason, and audit log.
- Distance shown to users is rounded/bucketed.
- Creating Echoes/Stitches requires proximity to Place:
  - Default radius: 100m adaptive by Place type/boundary.
  - Location proof must be fresh within 2 minutes.
  - GPS horizontal accuracy threshold: 100m.
  - Ambiguous eligible Places require user selection.
  - Server issues creation token after validating auth, Place, boundary, rate limits, and trust.
- Upload retries from local outbox are allowed after leaving the Place if creation proof was captured.

## Discovery and navigation

Top-level iOS navigation:

1. Map
2. Explore
3. Activity
4. Profile

Map:

- Place markers first, not individual Echo pins.
- Marker shows Place name, Echo count, and activity pulse.

Explore:

- Search, Trending/Notable, subscribed Places, city/place discovery.
- Place search first.
- Echo text/transcript search scoped to Place or nearby area.

Activity:

- Notifications.
- Mentions.
- Replies/Stitches/Reactions.
- Moderation results.
- Upload/outbox status.
- Subscribed Place alerts.

Profile:

- Public identity and user's own public Echoes.
- Basic public profile: handle, display name, avatar, bio, public Echoes, basic counts.

## Ranking and feeds

- Nearby uses adaptive radius starting around 1-3 km in dense areas and expanding when sparse.
- Place pages default to Notable sort.
- Notability uses recency, locality, engagement velocity, trust/safety quality, and capped trust-weighted subscription boost.
- Place notability is dominated by recent safe Echo activity.
- Subscriptions can boost global ranking only through small capped trust-weighted signal.
- Bookmarks do not directly affect global ranking.
- Reported-but-not-limited content gets risk downrank only after trust/severity thresholds.
- Limited content is excluded from public feeds/search/notifications.
- Ranking labels should lightly explain why content appears, e.g. “Popular nearby”, “Recent”, “From a subscribed Place”.

## Subscriptions, bookmarks, and notifications

Place Subscriptions:

- Private by default.
- Can be created from anywhere.
- Enable Place alerts.

Bookmarks:

- Private bookmarks for Echoes and Places.
- Admin access limited to aggregate/audited investigation paths.

Notifications:

- APNs from backend worker.
- Server stores notification records for Activity tab.
- Core notifications include replies, Stitches, mentions, reactions, moderation/account events, upload status, and notable Place Echoes.
- Place alerts trigger for notable top-level Echoes at subscribed Places.
- Sensitive content does not trigger Place alerts.
- Sensitive direct notifications use generic previews.
- Notification previews are smart: title/actor for safe direct activity, generic for sensitive/moderation/privacy cases.
- Thread/Place mute controls.
- Normal notification retention: 90 days.

## Media behavior

- Media upload uses presigned direct upload.
- Originals/private uploads stay private until approved.
- Public approved derivatives use stable CDN URLs.
- Private/original/pending media uses short-lived signed URLs.
- Object keys are opaque and do not expose user/place IDs.
- Internal exact-hash duplicate detection is allowed for storage/abuse, but not user-visible.
- Media processing before publication:
  - Normalize/compress.
  - Strip metadata.
  - Generate thumbnails.
  - Generate audio waveforms.
  - Generate transcripts/captions for audio/video when possible.
  - Scan/moderate.
- Originals retained briefly, then deleted after processing/retry/moderation window.
- Photo/video EXIF/GPS metadata is stripped.
- Library uploads allowed only with current valid location proof.
- Capture source is stored privately, not shown publicly.
- Muted video autoplay is enabled by default only on Wi-Fi.
- Data saver controls required.

## Moderation and safety

Baseline:

- 13+ moderated public app.
- Post immediately only after required automated checks; media can remain pending.
- If media scan is unavailable, accept upload into pending state, do not publish, alert admins.
- Local/open-source conservative moderation scanning is required because moderation must be free.
- Separate moderation host preferred.
- If separate moderation host is offline: media remains pending and admins are alerted; text can publish if text scan is available.

Moderation states:

- draft/local
- uploading
- processing
- pending review
- published
- limited
- removed
- deleted
- failed

Limited means author + moderators only, excluded from public feeds/search/notifications.

Reports:

- Report + block + hide are required.
- Reports are anonymous to reported user.
- Report categories: harassment, hate, sexual content, violence/threat, illegal/dangerous, privacy/location safety, spam/scam, misinformation, sensitive place, intellectual property, other.
- Reporting hides content for reporter immediately.
- Multiple trusted high-severity reports can auto-limit globally pending review.

Admin/moderation:

- Roles: user, moderator, admin, service.
- One moderator can act immediately; admin review/audit required for bans and sensitive-place removals during beta.
- Structured reason codes for removals/limits/sanctions.
- Authors see category + explanation + appeal option.
- Public viewers see generic removed placeholders only where continuity requires.
- Simple appeal flow for removed/limited content/account sanctions.
- Successful appeal restores content state/ranking, but missed notifications are not backfilled.
- Sanctions: warn, restrict posting, suspend, ban.
- Banned users' public content is removed by default.
- Admin safeguards: least privilege, audit, confirmations for bulk/destructive actions.
- Limited safe bulk moderation actions are allowed.

Sensitive content:

- User self-label + moderator override.
- Sensitive content can rank normally but is covered until tap-to-view.
- Sensitive content does not trigger Place alerts.

Blocking:

- Blocked users cannot reply/stitch/react to your content.
- You stop seeing their content in-app.
- The product does not promise true public invisibility for public content.
- Existing interactions are hidden from blocker but retained for moderation.

## Auth, privacy, and accounts

Auth:

- Sign in with Apple + passwordless email.
- Email UX: code + universal link.
- Email provider behind internal provider interface.
- Access token + refresh token stored in Keychain.
- Multi-device sessions with per-device APNs tokens and logout controls.
- No manual account recovery in beta.
- One public profile per account.

Privacy:

- GDPR-grade controls before EU invites.
- Versioned consent records for terms/privacy/community guidelines/hosting disclosure.
- Data export async job.
- Account deletion: deactivate immediately, remove public profile/content promptly, retain limited audit/moderation data for retention window, then purge personal/media data by policy.
- Deleted-user Echoes are removed from public view.
- Moderation/audit retention window: 90 days.
- Exact private creation coordinates retained while Echo is public plus moderation window after deletion/removal.
- Author can see exact location privately; public preview stays jittered.

Analytics:

- First-party minimal analytics only.
- User opt-out.
- No ad trackers.
- Analytics location context uses Place ID/coarse region only, never exact coordinates.
- Raw analytics events retained 90 days, then aggregate/anonymize.

## Public web and admin web

Public web:

- Minimal preview web app.
- Share links show preview/login CTA, not full public browsing.
- Legal/policy pages live here.

Admin web:

- Separate React/Vite admin frontend.
- Same user identity system with admin/moderator role.
- Admin features: users, Echoes, Replies, Stitches, reports, Places, moderation states, invite codes, media jobs, appeals, audit logs.

## Visual and UX direction

- Redesign from scratch.
- Current generated iOS app is reference only.
- Visual personality: civic map journal — calm, trustworthy, place-centered, readable in daylight, subtle story warmth.
- Support system light/dark mode.
- Accessibility is a release blocker:
  - Dynamic Type.
  - VoiceOver labels.
  - Captions/transcripts.
  - Sufficient contrast.
  - Reduced motion.
  - Non-audio alternatives.
- English beta, i18n-ready.
- User-generated content can be any language.
- Non-English transcription is best-effort local.

## Explicitly out of beta scope

- User following feed.
- Direct messages.
- Broad user search.
- Full public web browsing.
- Payments, ads, tipping, monetization.
- ML personalization.
- Advanced creator/business tools.
- Android.
- Place/business claim flows.
- General verification badges.
- Media replacement after publishing.

## Build stance

- Generated iOS app is disposable reference only.
- Start clean and port concepts selectively.
- Monorepo with `/apps/ios`, `/apps/api`, `/apps/admin-web`, `/apps/public-web`, `/docs`.
- First coding task after docs: scaffold repo, CI skeleton, placeholders, and remove invalid generated artifacts.
